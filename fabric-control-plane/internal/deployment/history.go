package deployment

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"sathonate.local/fabric-control-plane/internal/localfabric"
	"sathonate.local/fabric-control-plane/internal/operations"
)

// OpenHistoryDatabase takes a scoped read-only snapshot. It never opens an
// executor, acquires its lease, recovers jobs, or contacts Kubernetes/Docker.
func OpenHistoryDatabase(ctx context.Context, pool *pgxpool.Pool, scopes []string) (*Manager, error) {
	lifecycle, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: lifecycle, cancel: cancel, runner: historyRunner{}, history: map[string][]byte{}}
	items := []operations.Operation{}
	for _, scope := range scopes {
		if scope == "" {
			cancel()
			return nil, errors.New("authorized history scope required")
		}
		tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
		if err != nil {
			cancel()
			return nil, err
		}
		if _, err = tx.Exec(ctx, "SELECT set_config('sathonate.tenant_id',$1,true)", scope); err != nil {
			_ = tx.Rollback(ctx)
			cancel()
			return nil, err
		}
		rows, err := tx.Query(ctx, "SELECT o.payload,d.payload FROM sathonate_operations o LEFT JOIN sathonate_deployments d ON d.tenant_id=o.tenant_id AND d.operation_id=o.id WHERE o.tenant_id=$1", scope)
		if err != nil {
			_ = tx.Rollback(ctx)
			cancel()
			return nil, err
		}
		for rows.Next() {
			var opData, result []byte
			if err = rows.Scan(&opData, &result); err != nil {
				break
			}
			var op operations.Operation
			if err = json.Unmarshal(opData, &op); err != nil {
				break
			}
			if op.TenantID != scope {
				err = errors.New("history journal ownership mismatch")
				break
			}
			items = append(items, op)
			if len(result) > 0 {
				m.history[scope+"\x00"+op.ID] = append([]byte(nil), result...)
			}
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		_ = tx.Rollback(ctx)
		if err != nil {
			cancel()
			return nil, err
		}
	}
	m.jobs = operations.ReadOnlySnapshot(items)
	return m, nil
}

type historyRunner struct{}

func (historyRunner) Mode() string { return "history-only" }
func (historyRunner) Run(context.Context, Request, func(string, string) error) (Outcome, error) {
	return Outcome{}, operations.ErrReadOnly
}
func (historyRunner) Health(_ context.Context, _, _ string, previous *localfabric.Health) (localfabric.Health, error) {
	if previous == nil {
		return localfabric.Health{}, errors.New("no recorded health")
	}
	return *previous, nil
}
func (historyRunner) Logs(context.Context, string, string, string) (string, error) {
	return "", operations.ErrReadOnly
}
func (historyRunner) Cleanup(context.Context, string, string) error { return operations.ErrReadOnly }
