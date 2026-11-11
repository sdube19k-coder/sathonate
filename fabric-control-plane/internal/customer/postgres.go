package customer

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const SQLSchema = `
CREATE TABLE IF NOT EXISTS public.sathonate_customer_accounts(
 tenant_id text PRIMARY KEY, provider_ref text UNIQUE, payload jsonb NOT NULL
);
CREATE TABLE IF NOT EXISTS public.sathonate_customer_changes(
 tenant_id text NOT NULL REFERENCES public.sathonate_customer_accounts(tenant_id),
 change_id text NOT NULL, action text NOT NULL, digest text NOT NULL,
 actor_sha256 text NOT NULL, at timestamptz NOT NULL,
 PRIMARY KEY(tenant_id,change_id)
);
ALTER TABLE public.sathonate_customer_accounts ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.sathonate_customer_accounts FORCE ROW LEVEL SECURITY;
ALTER TABLE public.sathonate_customer_changes ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.sathonate_customer_changes FORCE ROW LEVEL SECURITY;
REVOKE ALL ON public.sathonate_customer_accounts,public.sathonate_customer_changes FROM PUBLIC;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_policies WHERE schemaname='public' AND tablename='sathonate_customer_accounts' AND policyname='customer_tenant') THEN
  CREATE POLICY customer_tenant ON public.sathonate_customer_accounts USING(tenant_id=current_setting('sathonate.customer_tenant',true)) WITH CHECK(tenant_id=current_setting('sathonate.customer_tenant',true));
 END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_policies WHERE schemaname='public' AND tablename='sathonate_customer_changes' AND policyname='customer_tenant') THEN
  CREATE POLICY customer_tenant ON public.sathonate_customer_changes USING(tenant_id=current_setting('sathonate.customer_tenant',true)) WITH CHECK(tenant_id=current_setting('sathonate.customer_tenant',true));
 END IF;
END $$;
`

type Postgres struct{ Pool *pgxpool.Pool }

// Ready rejects a pilot database role with account-bootstrap or audit-edit grants.
func (p Postgres) Ready(ctx context.Context) error {
	var safe bool
	err := p.Pool.QueryRow(ctx, `SELECT
      (SELECT count(*)=2 FROM pg_class WHERE oid IN (to_regclass('public.sathonate_customer_accounts'),to_regclass('public.sathonate_customer_changes')) AND relrowsecurity AND relforcerowsecurity)
      AND has_table_privilege(current_user,'public.sathonate_customer_accounts','SELECT')
      AND has_column_privilege(current_user,'public.sathonate_customer_accounts','payload','UPDATE')
      AND has_table_privilege(current_user,'public.sathonate_customer_changes','SELECT')
      AND has_table_privilege(current_user,'public.sathonate_customer_changes','INSERT')
      AND NOT has_table_privilege(current_user,'public.sathonate_customer_accounts','INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER')
      AND NOT has_column_privilege(current_user,'public.sathonate_customer_accounts','provider_ref','UPDATE')
      AND NOT has_column_privilege(current_user,'public.sathonate_customer_accounts','tenant_id','UPDATE')
      AND NOT has_table_privilege(current_user,'public.sathonate_customer_changes','DELETE,TRUNCATE,REFERENCES,TRIGGER')
      AND NOT has_any_column_privilege(current_user,'public.sathonate_customer_changes','UPDATE')
      AND NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='sathonate_customer_admin' AND pg_has_role(current_user,oid,'MEMBER'))`).Scan(&safe)
	if err != nil || !safe {
		return ErrUnavailable
	}
	return nil
}

func (p Postgres) RequireAdministrator(ctx context.Context) error {
	var allowed bool
	err := p.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='sathonate_customer_admin' AND pg_has_role(current_user,oid,'MEMBER'))`).Scan(&allowed)
	if err != nil || !allowed {
		return ErrDenied
	}
	return nil
}

func (p Postgres) transaction(ctx context.Context, tenant string, readOnly bool) (pgx.Tx, error) {
	if p.Pool == nil || !idPattern.MatchString(tenant) {
		return nil, ErrDenied
	}
	options := pgx.TxOptions{}
	if readOnly {
		options.AccessMode = pgx.ReadOnly
	}
	tx, err := p.Pool.BeginTx(ctx, options)
	if err != nil {
		return nil, ErrUnavailable
	}
	if _, err = tx.Exec(ctx, "SELECT set_config('sathonate.customer_tenant',$1,true)", tenant); err != nil {
		tx.Rollback(context.Background())
		return nil, ErrUnavailable
	}
	return tx, nil
}

func load(ctx context.Context, tx pgx.Tx, query, tenant string) (Account, error) {
	var account Account
	var raw []byte
	var reference *string
	err := tx.QueryRow(ctx, query, tenant).Scan(&raw, &reference)
	if errors.Is(err, pgx.ErrNoRows) {
		return account, ErrDenied
	}
	if err != nil || json.Unmarshal(raw, &account) != nil {
		return Account{}, ErrUnavailable
	}
	if reference != nil {
		account.ProviderRef = *reference
	}
	return account, nil
}

func (p Postgres) Read(ctx context.Context, tenant string) (Account, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := p.transaction(ctx, tenant, true)
	if err != nil {
		return Account{}, err
	}
	defer tx.Rollback(context.Background())
	return load(ctx, tx, "SELECT payload,provider_ref FROM public.sathonate_customer_accounts WHERE tenant_id=$1", tenant)
}

func (p Postgres) Update(ctx context.Context, tenant string, change Change, apply func(*Account) error) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := p.transaction(ctx, tenant, false)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	account, err := load(ctx, tx, "SELECT payload,provider_ref FROM public.sathonate_customer_accounts WHERE tenant_id=$1 FOR UPDATE", tenant)
	if err != nil {
		return err
	}
	var existing string
	err = tx.QueryRow(ctx, "SELECT digest FROM public.sathonate_customer_changes WHERE tenant_id=$1 AND change_id=$2", tenant, change.ID).Scan(&existing)
	if err == nil {
		if existing != change.Digest {
			return ErrConflict
		}
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ErrUnavailable
	}
	if err := apply(&account); err != nil {
		return err
	}
	account.ProviderRef = ""
	raw, err := json.Marshal(account)
	if err != nil {
		return ErrUnavailable
	}
	if _, err = tx.Exec(ctx, "UPDATE public.sathonate_customer_accounts SET payload=$2 WHERE tenant_id=$1", tenant, raw); err != nil {
		return ErrUnavailable
	}
	if _, err = tx.Exec(ctx, "INSERT INTO public.sathonate_customer_changes(tenant_id,change_id,action,digest,actor_sha256,at) VALUES($1,$2,$3,$4,$5,$6)",
		tenant, change.ID, change.Action, change.Digest, change.ActorSHA256, change.At); err != nil {
		return ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return ErrUnavailable
	}
	return nil
}

// Bootstrap is for a separately provisioned administrator connection. The
// runtime account receives no INSERT grant on customer accounts.
func (p Postgres) Bootstrap(ctx context.Context, tenant, name string, owner Identity, reference string) error {
	if p.RequireAdministrator(ctx) != nil {
		return ErrDenied
	}
	if len(name) < 2 || len(name) > 100 || !validIssuer(owner.Issuer) || len(owner.Subject) == 0 || len(owner.Subject) > 255 ||
		(reference != "" && !idPattern.MatchString(reference)) {
		return ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := p.transaction(ctx, tenant, false)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	account := Account{Name: name, Owner: owner}
	raw, err := json.Marshal(account)
	if err != nil {
		return ErrUnavailable
	}
	tag, err := tx.Exec(ctx, "INSERT INTO public.sathonate_customer_accounts(tenant_id,provider_ref,payload) VALUES($1,NULLIF($2,''),$3) ON CONFLICT(tenant_id) DO NOTHING", tenant, reference, raw)
	if err != nil {
		return ErrUnavailable
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	if _, err = tx.Exec(ctx, "INSERT INTO public.sathonate_customer_changes(tenant_id,change_id,action,digest,actor_sha256,at) VALUES($1,'bootstrap','bootstrap',$2,$3,now())", tenant, digest(account), digest(owner)); err != nil {
		return ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return ErrUnavailable
	}
	return nil
}
