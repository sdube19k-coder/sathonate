package deployment

import (
	"context"
	"encoding/json"
	"errors"
	"sathonate.local/fabric-control-plane/internal/operations"
	"time"
)

func (m *Manager) recoverCloud() error {
	if err := m.jobs.DiscoverCloud(m.ctx); err != nil {
		return err
	}
	for _, op := range m.jobs.RunningCloud() {
		m.mu.Lock()
		_, active := m.running[op.ID]
		m.mu.Unlock()
		if active {
			continue
		}
		var raw []byte
		if err := m.pool.QueryRow(m.ctx, "SELECT payload FROM sathonate_deployments WHERE tenant_id=$1 AND operation_id=$2", op.TenantID, op.ID).Scan(&raw); err != nil {
			return errors.New("cannot recover durable deployment input")
		}
		var snapshot Result
		if json.Unmarshal(raw, &snapshot) != nil {
			return errors.New("invalid durable deployment input")
		}
		snapshot.TenantID = op.TenantID
		// A journal can contain another runner's historical/in-flight work.
		// Never replay or cancel it through a newly selected execution adapter.
		if snapshot.Mode != m.Mode() {
			continue
		}
		if m.Mode() == "secure-eks" {
			validator, ok := m.runner.(interface{ Validate(Request) error })
			if !ok || validator.Validate(Request{TenantID: op.TenantID, NetworkID: op.NetworkID, Network: snapshot.Network, TargetSHA256: snapshot.TargetSHA256}) != nil {
				continue
			}
		}
		if op.CancelRequested {
			if err := m.Cancel(m.ctx, op.TenantID, op.ID); err != nil {
				return err
			}
			continue
		}
		if err := m.jobs.Replay(op.ID); err != nil {
			return err
		}
		m.mu.Lock()
		m.dispatch(op, snapshot, "")
		m.mu.Unlock()
	}
	return nil
}

// A single database owner is intentional. Loss of the session lock stops all
// command contexts; its successor adopts the same deterministic Kubernetes Jobs.
func (m *Manager) watchLease() {
	defer m.workers.Done()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(m.ctx, time.Second)
			err := m.lease.Ping(ctx)
			cancel()
			if err != nil {
				m.cancel()
				return
			}
			_ = m.recoverCloud() // Durable inputs remain pending after transient errors.
		}
	}
}

func (m *Manager) Cancel(ctx context.Context, tenant, id string) error {
	if m.history != nil {
		return operations.ErrReadOnly
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	op, err := m.jobs.Get(tenant, id)
	if err != nil {
		return err
	}
	if op.Status == "cancelled" {
		return nil
	}
	if op.Status != "running" {
		return errors.New("only running deployments can be cancelled")
	}
	runner, ok := m.runner.(interface {
		CancelDeployment(context.Context, Request) error
	})
	if !ok {
		return errors.New("cancellation is available for the EKS qualification runner")
	}
	snapshot, err := m.Status(tenant, id)
	if err != nil {
		return err
	}
	if err = m.jobs.RequestCancel(id); err != nil {
		return err
	}
	if cancel := m.running[id]; cancel != nil {
		cancel()
	}
	if err = runner.CancelDeployment(ctx, Request{TenantID: tenant, NetworkID: op.NetworkID, OperationID: id, Network: snapshot.Network}); err != nil {
		return err
	}
	return m.jobs.Cancelled(id)
}
