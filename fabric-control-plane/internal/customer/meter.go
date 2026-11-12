package customer

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"sathonate.local/fabric-control-plane/internal/identity"
)

const meterPageSize = 100

type MeterSource interface {
	Page(ctx context.Context, scope, after string, since, until time.Time) ([]VerifiedOperation, error)
}
type MeterReport struct {
	Reviewed     int `json:"reviewed"`
	Acknowledged int `json:"acknowledged"`
}
type Meter struct {
	Service Service
	Source  MeterSource
}

// Sweep revisits a bounded time window, so a job that crashes between source
// read and billing commit can restart at the beginning. The billing transaction
// deduplicates acknowledged operation IDs, including concurrent job attempts.
func (m Meter) Sweep(ctx context.Context, tenant, project string, window time.Duration) (MeterReport, error) {
	var report MeterReport
	if m.Source == nil || m.Service.Store == nil || !idPattern.MatchString(tenant) || !idPattern.MatchString(project) ||
		window < time.Hour || window > 30*24*time.Hour {
		return report, ErrInvalid
	}
	account, err := m.Service.Store.Read(ctx, tenant)
	if err != nil {
		return report, err
	}
	active := false
	for _, entry := range account.Projects {
		active = active || (entry.ID == project && entry.State == "active")
	}
	if !active {
		return report, ErrDenied
	}
	until := m.Service.now()
	since := until.Add(-window)
	scope := (identity.Principal{TenantID: tenant, ProjectID: project}).StorageTenant()
	cursor := ""
	for {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		page, err := m.Source.Page(ctx, scope, cursor, since, until)
		if err != nil || len(page) > meterPageSize {
			return report, ErrUnavailable
		}
		for _, entry := range page {
			if entry.ID <= cursor || entry.FinishedAt.Before(since) || entry.FinishedAt.After(until) {
				return report, ErrUnavailable
			}
			report.Reviewed++
			if err = m.Service.RecordVerifiedOperation(ctx, tenant, project, entry); err != nil {
				return report, err
			}
			report.Acknowledged++
			cursor = entry.ID
		}
		if len(page) < meterPageSize {
			return report, nil
		}
	}
}

// Page selects only fields needed for billing. It never loads deployment
// settings, Secret references, transaction contents, events or diagnostics.
func (p Postgres) Page(ctx context.Context, scope, after string, since, until time.Time) ([]VerifiedOperation, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := p.Pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, ErrUnavailable
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SELECT set_config('sathonate.tenant_id',$1,true)", scope); err != nil {
		return nil, ErrUnavailable
	}
	rows, err := tx.Query(ctx, `
       SELECT o.id,o.tenant_id,o.status,(o.payload->>'finishedAt')::timestamptz,
              d.payload->>'transactionId', d.payload->>'mode'
       FROM public.sathonate_operations o
       JOIN public.sathonate_deployments d ON d.tenant_id=o.tenant_id AND d.operation_id=o.id
       WHERE o.tenant_id=$1 AND o.id COLLATE "C">$2 AND o.status='succeeded'
          AND d.payload->>'mode'='customer-kubernetes'
          AND o.payload->>'operationId'=o.id AND o.payload->>'tenantId'=o.tenant_id
          AND d.payload->>'operationId'=o.id AND d.payload->>'phase'='succeeded'
          AND d.payload->'verified'='true'::jsonb AND d.payload->'ready'='true'::jsonb
          AND d.payload->'simulated'='false'::jsonb
          AND (o.payload->>'finishedAt')::timestamptz BETWEEN $3 AND $4
       ORDER BY o.id COLLATE "C" LIMIT $5`, scope, after, since, until, meterPageSize)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	var result []VerifiedOperation
	for rows.Next() {
		entry := VerifiedOperation{Verified: true, Ready: true}
		if err = rows.Scan(&entry.ID, &entry.Scope, &entry.Status, &entry.FinishedAt, &entry.TransactionID, &entry.Mode); err != nil {
			return nil, ErrUnavailable
		}
		result = append(result, entry)
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	return result, nil
}

// MeterReady requires a restricted source reader. In particular the metering
// job cannot create the execution evidence that it consumes.
func (p Postgres) MeterReady(ctx context.Context) error {
	if err := p.Ready(ctx); err != nil {
		return err
	}
	var safe bool
	err := p.Pool.QueryRow(ctx, `SELECT count(*)=2 AND coalesce(bool_and(
       relrowsecurity AND relforcerowsecurity AND has_table_privilege(current_user,oid,'SELECT')
       AND NOT has_table_privilege(current_user,oid,'INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER')
       AND NOT has_any_column_privilege(current_user,oid,'INSERT,UPDATE,REFERENCES')),false)
       AND NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='sathonate_executor' AND pg_has_role(current_user,oid,'MEMBER'))
       FROM pg_class WHERE oid IN(to_regclass('public.sathonate_operations'),to_regclass('public.sathonate_deployments'))`).Scan(&safe)
	if err != nil || !safe {
		return errors.New("restricted customer metering source unavailable")
	}
	return nil
}
