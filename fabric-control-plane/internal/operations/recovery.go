package operations

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Discover committed submissions after an ambiguous connection/commit result.
// Existing in-memory records are never overwritten by a stale database read.
func (s *Store) DiscoverCloud(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, "SELECT payload FROM sathonate_operations WHERE status='running' AND payload->>'kind' IN ('aws-eks-deploy','secure-local-deploy','secure-eks-deploy')")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var d diskRecord
		if err = rows.Scan(&raw); err != nil {
			return err
		}
		if err = json.Unmarshal(raw, &d); err != nil {
			return err
		}
		o := d.Operation
		o.IdempotencyKey = d.Key
		s.mu.Lock()
		if _, exists := s.items[o.ID]; !exists {
			s.items[o.ID] = o
		}
		s.mu.Unlock()
	}
	return rows.Err()
}

func (s *Store) persistPrepared(o Operation, payload []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	raw, e := json.Marshal(diskRecord{o, o.IdempotencyKey})
	if e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO sathonate_operations(tenant_id,id,network_id,status,idempotency_key,payload) VALUES($1,$2,$3,$4,$5,$6)`, o.TenantID, o.ID, o.NetworkID, o.Status, o.IdempotencyKey, raw); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO sathonate_deployments(tenant_id,operation_id,payload) VALUES($1,$2,$3)`, o.TenantID, o.ID, payload); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (s *Store) RunningCloud() []Operation {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Operation{}
	for _, o := range s.items {
		if (o.Kind == "aws-eks-deploy" || o.Kind == "secure-local-deploy" || o.Kind == "secure-eks-deploy") && o.Status == "running" {
			out = append(out, clone(o))
		}
	}
	return out
}
func (s *Store) RequestCancel(id string) error {
	return s.mutate(id, func(o *Operation) error {
		o.CancelRequested = true
		appendEvent(o, "Cancellation requested; waiting for worker termination")
		return nil
	})
}

// Replay events from the existing Job after a controller restart. Resetting only
// presentation stages does not rerun or replace that Kubernetes Job.
func (s *Store) Replay(id string) error {
	return s.mutate(id, func(o *Operation) error {
		if o.Kind != "aws-eks-deploy" && o.Kind != "secure-local-deploy" && o.Kind != "secure-eks-deploy" {
			return errors.New("only cloud Jobs support adoption")
		}
		o.CurrentStage = ""
		for i := range o.Stages {
			o.Stages[i].Status = "pending"
			o.Stages[i].StartedAt = nil
			o.Stages[i].FinishedAt = nil
		}
		appendEvent(o, "Controller recovered operation; adopting its existing Kubernetes Job and replaying observed events")
		return nil
	})
}
func (s *Store) Cancelled(id string) error {
	return s.mutate(id, func(o *Operation) error {
		now := time.Now().UTC()
		o.Status = "cancelled"
		o.FinishedAt = &now
		for i := range o.Stages {
			if o.Stages[i].Status == "running" || o.Stages[i].Status == "pending" {
				o.Stages[i].Status = "cancelled"
				o.Stages[i].FinishedAt = &now
			}
		}
		appendEvent(o, "Cancellation confirmed; worker Job stopped; Fabric data retained")
		return nil
	})
}
