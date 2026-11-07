package deployment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"path/filepath"
	"sync"
	"time"

	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/identity"
	"sathonate.local/fabric-control-plane/internal/kubernetesfabric"
	"sathonate.local/fabric-control-plane/internal/localfabric"
	"sathonate.local/fabric-control-plane/internal/maintenance"
	"sathonate.local/fabric-control-plane/internal/operations"
)

type Manager struct {
	history map[string][]byte
	root    string
	jobs    *operations.Store
	runner  Runner
	ctx     context.Context
	cancel  context.CancelFunc
	workers sync.WaitGroup
	mu      sync.Mutex
	pool    *pgxpool.Pool
	lease   *pgxpool.Conn
	running map[string]context.CancelFunc
}

func Open(root string, runner Runner) (*Manager, error) {
	if runner == nil || runner.Mode() != "mock" {
		return nil, errors.New("only the explicitly simulated runner is available; real cloud execution is disabled")
	}
	jobs, err := operations.Open(filepath.Join(root, "operations"))
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{root: root, jobs: jobs, runner: runner, ctx: ctx, cancel: cancel}, nil
}
func OpenDatabase(ctx context.Context, pool *pgxpool.Pool, runner Runner) (*Manager, error) {
	if runner == nil || (runner.Mode() != "local-kubernetes" && runner.Mode() != "mock" && runner.Mode() != "aws-eks" && runner.Mode() != "secure-local" && runner.Mode() != "secure-eks") {
		return nil, errors.New("unsupported local execution runner")
	}
	lease, err := pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	var owned bool
	if err = lease.QueryRow(ctx, "SELECT pg_try_advisory_lock(718481029)").Scan(&owned); err != nil || !owned {
		lease.Release()
		return nil, errors.New("another local executor owns this database")
	}
	var jobs *operations.Store
	if runner.Mode() == "aws-eks" || runner.Mode() == "secure-local" || runner.Mode() == "secure-eks" {
		jobs, err = operations.OpenRecoverableDatabase(ctx, pool)
	} else {
		jobs, err = operations.OpenDatabase(ctx, pool)
	}
	if err != nil {
		_, _ = lease.Exec(ctx, "SELECT pg_advisory_unlock(718481029)")
		lease.Release()
		return nil, err
	}
	lifecycle, cancel := context.WithCancel(context.Background())
	m := &Manager{jobs: jobs, runner: runner, ctx: lifecycle, cancel: cancel, pool: pool, lease: lease, running: map[string]context.CancelFunc{}}
	if runner.Mode() == "aws-eks" || runner.Mode() == "secure-local" || runner.Mode() == "secure-eks" {
		if err = m.recoverCloud(); err != nil {
			m.Close()
			return nil, err
		}
		m.workers.Add(1)
		go m.watchLease()
	}
	return m, nil
}
func (m *Manager) Close() {
	m.cancel()
	m.workers.Wait()
	if m.lease != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = m.lease.Exec(ctx, "SELECT pg_advisory_unlock(718481029)")
		m.lease.Release()
	}
}
func (m *Manager) Operations() *operations.Store { return m.jobs }
func (m *Manager) Mode() string                  { return m.runner.Mode() }
func (m *Manager) Start(tenant, actor, record string, s domain.NetworkSpec, revision int, key, fail, retryOf string) (operations.Operation, error) {
	return m.start(tenant, actor, record, s, revision, key, fail, retryOf, nil, "", "", "", "")
}
func (m *Manager) start(tenant, actor, record string, s domain.NetworkSpec, revision int, key, fail, retryOf string, lifecycle *kubernetesfabric.LifecycleState, parent, action, expectedParent, approval string, inputs ...transitionInput) (operations.Operation, error) {
	intent, e := json.Marshal(s)
	if e != nil {
		return operations.Operation{}, errors.New("invalid desired configuration")
	}
	var isolated domain.NetworkSpec
	if json.Unmarshal(intent, &isolated) != nil {
		return operations.Operation{}, errors.New("invalid desired configuration")
	}
	s = isolated
	var previousNetwork *domain.NetworkSpec
	var recovery *NodeRecovery
	if len(inputs) == 1 && inputs[0].Recovery != nil {
		raw, _ := json.Marshal(inputs[0].Recovery)
		json.Unmarshal(raw, &recovery)
	}
	if len(inputs) > 1 {
		return operations.Operation{}, errors.New("invalid dynamic transition")
	}
	if len(inputs) == 1 && inputs[0].Previous != nil {
		raw, _ := json.Marshal(inputs[0].Previous)
		var copy domain.NetworkSpec
		if json.Unmarshal(raw, &copy) != nil {
			return operations.Operation{}, errors.New("invalid previous network")
		}
		previousNetwork = &copy
		if m.Mode() != "secure-local" || lifecycle != nil || (action != "dynamic-reconcile" && action != "node-recovery") || parent == "" || approval == "" || kubernetesfabric.ValidateDynamicTransition(copy, s) != nil {
			return operations.Operation{}, errors.New("invalid dynamic transition approval")
		}
	}
	if (recovery != nil && (previousNetwork == nil || action != "node-recovery" || recovery.Digest != approval)) || (action == "node-recovery" && recovery == nil) {
		return operations.Operation{}, errors.New("invalid recovery intent")
	}
	if m.history != nil {
		return operations.Operation{}, operations.ErrReadOnly
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if previousNetwork != nil {
		for _, op := range m.jobs.List(tenant) {
			if op.IdempotencyKey == key {
				stored, e := m.Status(tenant, op.ID)
				if e != nil || op.NetworkID != record || op.Actor != actor || stored.PreviousNetwork == nil || stored.LifecycleParent != parent || stored.LifecycleAction != action || stored.LifecycleApproval != approval {
					return operations.Operation{}, operations.ErrIdempotency
				}
				expected, _ := json.Marshal(s)
				actual, _ := json.Marshal(stored.Network)
				if !bytes.Equal(expected, actual) {
					return operations.Operation{}, operations.ErrIdempotency
				}
				return op, nil
			}
		}
	}
	if retryOf != "" {
		previous, e := m.Status(tenant, retryOf)
		if e != nil || previous.Phase == "superseded" {
			return operations.Operation{}, errors.New("retry the latest operation; this intent was superseded")
		}
	}
	if lifecycle != nil {
		copy := *lifecycle
		lifecycle = &copy
		for _, op := range m.jobs.List(tenant) {
			if op.IdempotencyKey == key {
				stored, e := m.Status(tenant, op.ID)
				if e != nil || op.NetworkID != record || op.Actor != actor || stored.LifecycleParent != parent || stored.LifecycleAction != action || stored.LifecycleApproval != approval {
					return operations.Operation{}, operations.ErrIdempotency
				}
				return op, nil
			}
		}
	}
	if lifecycle != nil && (m.Mode() != "secure-local" || lifecycle.Validate() != nil) {
		return operations.Operation{}, errors.New("lifecycle execution requires the bounded protected local runner")
	}
	if expectedParent != "" {
		previous, e := m.Status(tenant, expectedParent)
		if e != nil || !previous.Verified || previous.Phase != "running" {
			return operations.Operation{}, errors.New("change preview is stale")
		}
	}
	if lifecycle == nil {
		for _, previous := range m.jobs.List(tenant) {
			if previous.NetworkID == record {
				state, e := m.Status(tenant, previous.ID)
				if e == nil && state.LocalDeletion != nil {
					return operations.Operation{}, errors.New("network retired; automatic redeployment is disabled")
				}
				if previousNetwork == nil && e == nil && state.PreviousNetwork != nil {
					return operations.Operation{}, errors.New("use reviewed dynamic reconciliation for a managed expanded network")
				}
				if previousNetwork == nil && e == nil && (s.Spec.Deployment.TargetRef == domain.DynamicLocalProfile || state.Network.Spec.Deployment.TargetRef == domain.DynamicLocalProfile) {
					current, _ := json.Marshal(state.Network)
					desired, _ := json.Marshal(s)
					if !bytes.Equal(current, desired) {
						return operations.Operation{}, errors.New("dynamic running-network changes require qualified durable transition approval; initial deployment intent cannot replace membership")
					}
				}
				if e != nil || state.Lifecycle != nil {
					return operations.Operation{}, errors.New("use lifecycle reconciliation for a managed topology")
				}
				break
			}
		}
	}
	if err := rejectUnintegratedSecureProfile(s); err != nil && m.Mode() != "secure-local" && m.Mode() != "secure-eks" {
		return operations.Operation{}, err
	}
	if m.ctx.Err() != nil {
		return operations.Operation{}, errors.New("deployment manager stopped")
	}
	if revision < 1 || s.Spec.Deployment.Engine != "bevel" || domain.HasErrors(domain.Validate(s)) {
		return operations.Operation{}, errors.New("save a valid cloud design before deployment")
	}
	if fail != "" {
		if m.Mode() != "mock" {
			return operations.Operation{}, errors.New("failure injection is restricted to isolated mock tests")
		}
		found := false
		for _, stage := range Stages {
			if fail == stage.Key {
				found = true
			}
		}
		if !found {
			return operations.Operation{}, errors.New("unknown mock failure stage")
		}
	}
	material, _ := json.Marshal(struct {
		Network        domain.NetworkSpec
		Revision       int
		Lifecycle      *kubernetesfabric.LifecycleState
		Parent, Action string
	}{s, revision, lifecycle, parent, action})
	if lifecycle == nil {
		material, _ = json.Marshal(struct {
			Network  domain.NetworkSpec
			Revision int
		}{s, revision})
	}
	if previousNetwork != nil {
		material, _ = json.Marshal([]any{s, revision, previousNetwork, parent, approval})
		if recovery != nil {
			material, _ = json.Marshal([]any{s, revision, previousNetwork, parent, approval, recovery})
		}
	}
	hash := sha256.Sum256(material)
	if m.Mode() == "local-kubernetes" && (s.Spec.Deployment.Provider != "local" || s.Spec.Deployment.TargetRef != "smp-qualification" || s.Spec.Environment != "development") {
		return operations.Operation{}, errors.New("this worker executes only the explicit local Kubernetes target")
	}
	if validator, ok := m.runner.(interface{ Validate(Request) error }); ok {
		if err := validator.Validate(Request{TenantID: tenant, NetworkID: record, Network: s, Lifecycle: lifecycle, PreviousNetwork: previousNetwork, Recovery: recovery, LifecycleAction: action, LifecycleApproval: approval}); err != nil {
			return operations.Operation{}, err
		}
	}
	targetDigest := ""
	if binding, ok := m.runner.(interface{ BindingDigest(Request) string }); ok {
		targetDigest = binding.BindingDigest(Request{TenantID: tenant, NetworkID: record, Network: s})
	}
	begin := m.jobs.Begin
	if m.Mode() == "aws-eks" || m.Mode() == "secure-local" || m.Mode() == "secure-eks" {
		begin = func(tenant, actor, record, kind, key, digest, version, retry string, stages []operations.Stage) (operations.Operation, bool, error) {
			return m.jobs.BeginPrepared(tenant, actor, record, kind, key, digest, version, retry, stages, func(op operations.Operation) ([]byte, error) {
				return json.Marshal(Result{Recovery: recovery, PreviousNetwork: previousNetwork, Lifecycle: lifecycle, LifecycleParent: parent, LifecycleAction: action, LifecycleApproval: approval, TenantID: tenant, TargetSHA256: targetDigest, Mode: m.Mode(), Network: s, Revision: revision, OperationID: op.ID, Phase: "deploying", CheckedAt: time.Now().UTC(), Resources: []Resource{}, Notice: "Durable target-bound qualification deployment; cloud infrastructure provisioning is excluded."})
			})
		}
	}
	op, reused, err := begin(tenant, actor, record, m.Mode()+"-deploy", key, hex.EncodeToString(hash[:]), s.Spec.FabricVersion, retryOf, Stages)
	if err != nil || reused {
		return op, err
	}
	snapshot := Result{Recovery: recovery, PreviousNetwork: previousNetwork, Lifecycle: lifecycle, LifecycleParent: parent, LifecycleAction: action, LifecycleApproval: approval, TargetSHA256: targetDigest, Mode: "mock", Simulated: true, Network: s, Revision: revision, OperationID: op.ID, Phase: "deploying", CheckedAt: time.Now().UTC(), Resources: []Resource{}, Notice: "MOCK DEPLOYMENT — simulated resources and transaction; no cloud, Kubernetes, CA or Fabric resources are created."}
	snapshot.TenantID = tenant
	if m.Mode() == "secure-local" {
		snapshot.Notice = "Explicit protected local candidate deployment; production and AWS are disabled."
	}
	if m.Mode() == "local-kubernetes" || m.Mode() == "aws-eks" || m.Mode() == "secure-local" || m.Mode() == "secure-eks" {
		snapshot.Mode = m.Mode()
		snapshot.Simulated = false
		snapshot.Notice = "Real local Kubernetes deployment using Bevel Fabric operator. No paid cloud infrastructure is provisioned."
	}
	if m.Mode() == "aws-eks" || m.Mode() == "secure-eks" {
		snapshot.Notice = "Real EKS qualification deployment using the pinned Bevel operator; no cloud infrastructure is provisioned."
	}
	if m.Mode() == "secure-eks" {
		snapshot.Notice = "Protected EKS qualification only. Cluster/IAM are provisioned separately; PVCs may allocate billable EBS. Production is not qualified."
	}
	if err = m.write(snapshot); err != nil {
		_ = m.jobs.Finish(op.ID, errors.New("could not persist deployment input"))
		return operations.Operation{}, err
	}
	m.dispatch(op, snapshot, fail)
	return op, nil
}
func (m *Manager) dispatch(op operations.Operation, snapshot Result, fail string) {
	if m.running == nil {
		m.running = map[string]context.CancelFunc{}
	}
	runCtx, cancel := context.WithCancel(m.ctx)
	m.running[op.ID] = cancel
	tenant, record, s := op.TenantID, op.NetworkID, snapshot.Network
	m.workers.Add(1)
	go func() {
		defer m.workers.Done()
		defer cancel()
		defer func() { m.mu.Lock(); delete(m.running, op.ID); m.mu.Unlock() }()
		outcome, runErr := m.runner.Run(runCtx, Request{TenantID: tenant, NetworkID: record, OperationID: op.ID, Network: s, Recovery: snapshot.Recovery, PreviousNetwork: snapshot.PreviousNetwork, Lifecycle: snapshot.Lifecycle, LifecycleAction: snapshot.LifecycleAction, LifecycleApproval: snapshot.LifecycleApproval, TargetSHA256: snapshot.TargetSHA256, FailureStage: fail}, func(stage, message string) error {
			if err := m.jobs.Advance(op.ID, stage); err != nil {
				return err
			}
			return m.jobs.Event(op.ID, message)
		})
		if runErr == nil {
			snapshot.Verifications = outcome.Verifications
			if m.Mode() != "mock" {
				runErr = verifyLifecycleEvidence(s, snapshot.Lifecycle, outcome)
			}
			if (m.Mode() == "secure-local" || m.Mode() == "secure-eks") && (outcome.Security == nil || !outcome.Security.CredentialsAbsent || outcome.Security.CACommands < 1 || outcome.Security.ProtectedCLICommands < 1) {
				runErr = errors.New("secure worker process argument evidence is incomplete")
			}
		}
		if (m.Mode() == "aws-eks" || m.Mode() == "secure-local" || m.Mode() == "secure-eks") && m.ctx.Err() != nil {
			return
		}
		latest, _ := m.jobs.Get(tenant, op.ID)
		if latest.CancelRequested || latest.Status == "cancelled" {
			return
		}
		if runErr == nil {
			snapshot.Resources = outcome.Resources
			snapshot.Security = outcome.Security
			snapshot.Phase = "running"
			snapshot.Ready = true
			snapshot.TransactionID = outcome.TransactionID
			if snapshot.Simulated {
				snapshot.TransactionID = "mock-tx-" + op.ID
			} else {
				snapshot.Verified = true
				snapshot.Health = outcome.Health
			}
			snapshot.CheckedAt = time.Now().UTC()
			runErr = m.write(snapshot)
		}
		_ = m.jobs.Finish(op.ID, runErr)
	}()
}

func (m *Manager) write(s Result) error {
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if m.history != nil {
		return operations.ErrReadOnly
	}
	if m.pool != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err = m.pool.Exec(ctx, `INSERT INTO sathonate_deployments(tenant_id,operation_id,payload) VALUES($1,$2,$3) ON CONFLICT(tenant_id,operation_id) DO UPDATE SET payload=EXCLUDED.payload,updated_at=now()`, s.TenantID, s.OperationID, raw)
		return err
	}
	path := filepath.Join(m.root, s.OperationID+".json")
	f, err := os.OpenFile(path+".tmp", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(raw); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(path+".tmp", path)
}
func (m *Manager) Status(tenant, id string) (Result, error) {
	op, err := m.jobs.Get(tenant, id)
	if err != nil {
		return Result{}, err
	}
	var raw []byte
	if m.history != nil {
		raw = m.history[tenant+"\x00"+op.ID]
		if len(raw) == 0 {
			return Result{}, operations.ErrNotFound
		}
	} else if m.pool != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err = m.pool.QueryRow(ctx, "SELECT payload FROM sathonate_deployments WHERE tenant_id=$1 AND operation_id=$2", tenant, op.ID).Scan(&raw)
	} else {
		raw, err = os.ReadFile(filepath.Join(m.root, op.ID+".json"))
	}
	if err != nil {
		return Result{}, err
	}
	var s Result
	if err = json.Unmarshal(raw, &s); err != nil {
		return Result{}, err
	}
	if op.Status != "succeeded" && s.Phase != "removed" {
		s.Ready = false
		s.Verified = false
		s.Phase = op.Status
	}
	if s.Simulated {
		s.Verified = false
	}
	for _, newer := range m.jobs.List(tenant) {
		if newer.NetworkID == op.NetworkID && newer.CreatedAt.After(op.CreatedAt) {
			s.Ready = false
			s.Verified = false
			s.Phase = "superseded"
			s.Notice = "Historical operation. Open the latest deployment operation for current network state."
			break
		}
	}
	if m.history != nil {
		s.Mode = "history-only"
		s.Ready = false
		s.Verified = false
		s.Notice = "Recorded history only. Live Fabric state was not queried; execution is disabled."
		if s.Health != nil {
			s.Health.Ready = false
			s.Health.Alerts = append(s.Health.Alerts, s.Notice)
		}
	}
	return s, nil
}
func (m *Manager) Retry(tenant, actor, id, key string) (operations.Operation, error) {
	if m.history != nil {
		return operations.Operation{}, operations.ErrReadOnly
	}
	s, err := m.Status(tenant, id)
	if err != nil {
		return operations.Operation{}, err
	}
	op, err := m.jobs.Get(tenant, id)
	if err != nil {
		return operations.Operation{}, err
	}
	if op.Status != "failed" && op.Status != "interrupted" {
		return operations.Operation{}, fmt.Errorf("only a failed or interrupted deployment can be retried")
	}
	if s.Phase == "superseded" {
		return operations.Operation{}, errors.New("retry the latest deployment operation instead of historical input")
	}
	if binding, ok := m.runner.(interface{ BindingDigest(Request) string }); ok && s.TargetSHA256 != binding.BindingDigest(Request{TenantID: tenant, NetworkID: op.NetworkID, Network: s.Network}) {
		return operations.Operation{}, errors.New("target configuration changed; restore the recorded target binding before retry")
	}
	if runner, ok := m.runner.(interface {
		CancelDeployment(context.Context, Request) error
	}); ok {
		if err := runner.CancelDeployment(m.ctx, Request{TenantID: tenant, NetworkID: op.NetworkID, OperationID: id, Network: s.Network}); err != nil {
			return operations.Operation{}, err
		}
	}
	if runner, ok := m.runner.(interface {
		Cancel(context.Context, string, string) error
	}); ok {
		if err := runner.Cancel(m.ctx, tenant, id); err != nil {
			return operations.Operation{}, err
		}
	}
	return m.start(tenant, actor, op.NetworkID, s.Network, s.Revision, key, "", op.ID, s.Lifecycle, s.LifecycleParent, s.LifecycleAction, "", s.LifecycleApproval, transitionInput{Previous: s.PreviousNetwork, Recovery: s.Recovery})
}

func (m *Manager) Health(ctx context.Context, tenant, id string) (localfabric.Health, error) {
	s, err := m.Status(tenant, id)
	if err != nil {
		return localfabric.Health{}, err
	}
	op, err := m.jobs.Get(tenant, id)
	if err != nil {
		return localfabric.Health{}, err
	}
	runner, ok := m.runner.(interface {
		Health(context.Context, string, string, *localfabric.Health) (localfabric.Health, error)
		Logs(context.Context, string, string, string) (string, error)
		Cleanup(context.Context, string, string) error
	})
	if !ok || s.Simulated {
		return localfabric.Health{}, errors.New("real Kubernetes health is unavailable")
	}
	return runner.Health(ctx, tenant, op.NetworkID, s.Health)
}
func (m *Manager) Logs(ctx context.Context, tenant, id, node string) (string, error) {
	s, err := m.Status(tenant, id)
	if err != nil {
		return "", err
	}
	found := false
	effective := s.Network
	if s.Lifecycle != nil {
		effective = s.Lifecycle.Effective(effective)
	}
	for _, n := range effective.Spec.Nodes {
		if n.ID == node {
			found = true
		}
	}
	if !found {
		return "", errors.New("unknown Fabric node")
	}
	op, err := m.jobs.Get(tenant, id)
	if err != nil {
		return "", err
	}
	runner, ok := m.runner.(interface {
		Health(context.Context, string, string, *localfabric.Health) (localfabric.Health, error)
		Logs(context.Context, string, string, string) (string, error)
		Cleanup(context.Context, string, string) error
	})
	if !ok {
		return "", errors.New("real node logs are unavailable")
	}
	return runner.Logs(ctx, tenant, op.NetworkID, node)
}
func (m *Manager) Cleanup(ctx context.Context, tenant, id string) error {
	return m.cleanup(ctx, tenant, id, "", "")
}
func (m *Manager) CleanupApproved(ctx context.Context, tenant, id, actor, digest string) error {
	return m.cleanup(ctx, tenant, id, actor, digest)
}
func (m *Manager) cleanup(ctx context.Context, tenant, id, actor, approval string) error {
	if m.history != nil {
		return operations.ErrReadOnly
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.Status(tenant, id)
	if err != nil {
		return err
	}
	if s.Phase == "superseded" {
		return errors.New("delete through the latest deployment operation")
	}
	op, err := m.jobs.Get(tenant, id)
	if err != nil {
		return err
	}
	for _, active := range m.jobs.List(tenant) {
		if active.NetworkID == op.NetworkID && active.Status == "running" {
			return errors.New("wait for the active operation before deleting this network")
		}
	}
	if m.Mode() == "secure-local" {
		return m.deleteLocal(ctx, tenant, id, actor, approval, s, op)
	}
	runner, ok := m.runner.(interface {
		Health(context.Context, string, string, *localfabric.Health) (localfabric.Health, error)
		Logs(context.Context, string, string, string) (string, error)
		Cleanup(context.Context, string, string) error
	})
	if !ok {
		return errors.New("real local cleanup is unavailable")
	}
	if m.Mode() == "aws-eks" || m.Mode() == "secure-eks" {
		approved, ok := m.runner.(interface {
			CleanupApproved(context.Context, Request, string) error
		})
		approvalBytes, approvalErr := hex.DecodeString(approval)
		if !ok || actor == "" || approvalErr != nil || len(approvalBytes) != 32 {
			return errors.New("administrator cleanup preview approval required")
		}
		actorSum := sha256.Sum256([]byte(actor))
		if err = m.jobs.Event(id, "Administrator requested cleanup; preview binding verification required; actor hash "+hex.EncodeToString(actorSum[:])); err != nil {
			return err
		}
		err = approved.CleanupApproved(ctx, Request{TenantID: tenant, NetworkID: op.NetworkID, OperationID: id, Network: s.Network, TargetSHA256: s.TargetSHA256}, approval)
		if err != nil {
			_ = m.jobs.Event(id, "Cleanup interrupted; re-inventory remaining resources before approval")
			return err
		}
		if err = m.jobs.Event(id, "Exact Fabric resources and worker access deletion verified; sensitive data retention remains an administrator responsibility"); err != nil {
			return err
		}
	} else if err = runner.Cleanup(ctx, tenant, op.NetworkID); err != nil {
		return err
	}
	s.TenantID = tenant
	s.Phase = "removed"
	for i := range s.Resources {
		s.Resources[i].Status = "removed"
	}
	if m.Mode() == "aws-eks" || m.Mode() == "secure-eks" {
		s.Notice = "Exact Fabric resources and worker access removed. Namespace, Secrets, PVCs and external backups were excluded; independently verify operator cascading and storage retention."
	}
	s.Ready = false
	s.Verified = false
	if s.Health != nil {
		s.Health.Ready = false
		s.Health.Alerts = []string{"Kubernetes Fabric workloads removed"}
	}
	return m.write(s)
}

func (m *Manager) CleanupPreview(ctx context.Context, tenant, id string) (CleanupPreview, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	snapshot, err := m.Status(tenant, id)
	if err != nil {
		return CleanupPreview{}, err
	}
	op, err := m.jobs.Get(tenant, id)
	if err != nil {
		return CleanupPreview{}, err
	}
	if snapshot.Phase == "superseded" {
		return CleanupPreview{}, errors.New("preview through the latest operation")
	}
	for _, active := range m.jobs.List(tenant) {
		if active.NetworkID == op.NetworkID && active.Status == "running" {
			return CleanupPreview{}, errors.New("active network operation prevents cleanup preview")
		}
	}
	runner, ok := m.runner.(interface {
		PreviewCleanup(context.Context, Request) (CleanupPreview, error)
	})
	if !ok {
		return CleanupPreview{}, errors.New("approved cleanup preview is available only for EKS qualification")
	}
	return runner.PreviewCleanup(ctx, Request{TenantID: tenant, NetworkID: op.NetworkID, OperationID: id, Network: snapshot.Network, TargetSHA256: snapshot.TargetSHA256})
}

func (m *Manager) PlanRotation(ctx context.Context, p identity.Principal, id, ref, key string) (maintenance.Intent, error) {
	if !p.CanAdminister() {
		return maintenance.Intent{}, maintenance.ErrDenied
	}
	snapshot, err := m.Status(p.TenantID, id)
	if err != nil {
		return maintenance.Intent{}, maintenance.ErrDenied
	}
	op, err := m.jobs.Get(p.TenantID, id)
	if err != nil {
		return maintenance.Intent{}, maintenance.ErrDenied
	}
	if m.Mode() != "aws-eks" || snapshot.TargetSHA256 == "" || snapshot.Phase == "superseded" {
		return maintenance.Intent{}, errors.New("rotation planning requires the latest approved target-bound deployment")
	}
	valid := false
	for _, org := range snapshot.Network.Spec.Organizations {
		valid = valid || ref == org.ID+"/admin" || ref == org.ID+"/registrar"
	}
	for _, node := range snapshot.Network.Spec.Nodes {
		valid = valid || ref == node.OrganizationRef+"/"+node.ID
		if node.Role == "peer" {
			valid = valid || ref == node.OrganizationRef+"/contract"
		}
	}
	if !valid {
		return maintenance.Intent{}, maintenance.ErrDenied
	}
	request := Request{TenantID: p.TenantID, NetworkID: op.NetworkID, Network: snapshot.Network, TargetSHA256: snapshot.TargetSHA256}
	validator, ok := m.runner.(interface{ Validate(Request) error })
	if !ok || validator.Validate(request) != nil {
		return maintenance.Intent{}, maintenance.ErrDenied
	}
	raw, _ := json.Marshal(snapshot.Network)
	intent, err := maintenance.NewIntent(p, maintenance.Binding{TenantID: p.TenantID, NetworkID: op.NetworkID, TargetSHA256: snapshot.TargetSHA256, NetworkSHA256: maintenance.HashBytes(raw)}, ref, key, time.Now())
	if err != nil {
		return maintenance.Intent{}, err
	}
	result, err := (maintenance.Store{Pool: m.pool}).Create(ctx, intent)
	if err != nil {
		return maintenance.Intent{}, err
	}
	if err = m.jobs.Event(id, "Explicit credential rotation plan recorded; live mutation adapter is disabled"); err != nil {
		return maintenance.Intent{}, err
	}
	return result, nil
}
func (m *Manager) RotationPlan(ctx context.Context, p identity.Principal, operation, id string) (maintenance.Intent, error) {
	if !p.CanAdminister() {
		return maintenance.Intent{}, maintenance.ErrDenied
	}
	snapshot, err := m.Status(p.TenantID, operation)
	if err != nil {
		return maintenance.Intent{}, maintenance.ErrDenied
	}
	op, err := m.jobs.Get(p.TenantID, operation)
	if err != nil {
		return maintenance.Intent{}, maintenance.ErrDenied
	}
	result, err := (maintenance.Store{Pool: m.pool}).Get(ctx, p.TenantID, id)
	if err != nil || result.Binding.NetworkID != op.NetworkID || result.Binding.TargetSHA256 != snapshot.TargetSHA256 {
		return maintenance.Intent{}, maintenance.ErrDenied
	}
	return result, nil
}
