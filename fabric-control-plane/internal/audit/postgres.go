package audit

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const SQLSchema = `
CREATE TABLE IF NOT EXISTS public.sathonate_audit_events (
 scope text NOT NULL CHECK (scope <> ''),
 request_id text NOT NULL CHECK (request_id ~ '^[a-f0-9]{32}$'),
 phase text NOT NULL CHECK (phase IN ('attempt','completion','interrupted')),
 occurred_at timestamptz NOT NULL,
 event jsonb NOT NULL,
 PRIMARY KEY (scope, request_id, phase)
);
CREATE INDEX IF NOT EXISTS sathonate_audit_scope_time ON public.sathonate_audit_events(scope, occurred_at DESC, request_id, phase);
ALTER TABLE public.sathonate_audit_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.sathonate_audit_events FORCE ROW LEVEL SECURITY;
DO $$ BEGIN
 IF NOT EXISTS (SELECT FROM pg_policies WHERE schemaname='public' AND tablename='sathonate_audit_events' AND policyname='sathonate_audit_read') THEN
  CREATE POLICY sathonate_audit_read ON public.sathonate_audit_events FOR SELECT USING(scope=current_setting('sathonate.audit_scope',true));
  CREATE POLICY sathonate_audit_append ON public.sathonate_audit_events FOR INSERT WITH CHECK(scope=current_setting('sathonate.audit_scope',true));
 END IF;
END $$;
`

type Postgres struct{ Pool *pgxpool.Pool }

func (s Postgres) Journal() *Journal { return &Journal{persist: s.append} }

// Ready verifies storage and grants before the service accepts requests.
func (s Postgres) Ready(ctx context.Context, restricted bool) error {
	var ready bool
	err := s.Pool.QueryRow(ctx, `SELECT relrowsecurity AND relforcerowsecurity
	 AND has_table_privilege(current_user,oid,'SELECT')
	 AND has_table_privilege(current_user,oid,'INSERT')
	 AND (NOT $1 OR NOT has_table_privilege(current_user,oid,'UPDATE,DELETE,TRUNCATE'))
	 FROM pg_class WHERE oid=to_regclass('public.sathonate_audit_events')`, restricted).Scan(&ready)
	if err != nil || !ready {
		return errors.New("audit table unavailable or privileges invalid")
	}
	return nil
}

func (s Postgres) transaction(ctx context.Context, scope string, readOnly bool) (pgx.Tx, error) {
	if scope == "" || s.Pool == nil {
		return nil, errors.New("audit project scope required")
	}
	options := pgx.TxOptions{}
	if readOnly {
		options.AccessMode = pgx.ReadOnly
	}
	tx, err := s.Pool.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('sathonate.audit_scope',$1,true)", scope); err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}

func (s Postgres) append(ctx context.Context, scope string, event Event) error {
	tx, err := s.transaction(ctx, scope, false)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO public.sathonate_audit_events
		(scope,request_id,phase,occurred_at,event) VALUES ($1,$2,$3,$4,$5)`,
		scope, event.RequestID, event.Phase, event.At, event); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s Postgres) Recent(ctx context.Context, scope string) ([]Event, error) {
	tx, err := s.transaction(ctx, scope, true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT event FROM public.sathonate_audit_events
		WHERE scope=$1 ORDER BY occurred_at DESC, request_id, phase LIMIT 100`, scope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []Event{}
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}
