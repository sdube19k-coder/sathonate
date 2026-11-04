package cloud

import (
	"context"
	"crypto/tls"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

// PostgreSQLReadiness is opt-in and read-only. Its error paths never return a
// connection string, server diagnostic, username or credential value.
func PostgreSQLReadiness(ctx context.Context, connection string) ReadinessCheck {
	result := ReadinessCheck{ID: "postgresql", Status: "NOT VERIFIED", Detail: "Explicit protected database configuration and verified TLS probe required", CheckedAt: time.Now().UTC()}
	if connection == "" {
		return result
	}
	cfg, err := securePostgresConfig(connection)
	if err != nil {
		result.Status = "BLOCKED"
		result.Detail = "Database configuration must enforce verified hostname TLS with no plaintext fallback"
		return result
	}
	deadline, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(deadline, cfg)
	if err != nil {
		result.Status = "BLOCKED"
		result.Detail = "Read-only database connection could not be established"
		return result
	}
	defer pool.Close()
	var super, bypass, encrypted bool
	var tables, rls int
	err = pool.QueryRow(deadline, `SELECT r.rolsuper,r.rolbypassrls,coalesce((SELECT ssl FROM pg_stat_ssl WHERE pid=pg_backend_pid()),false),
 (SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname IN ('sathonate_networks','sathonate_network_revisions','sathonate_operations','sathonate_deployments','sathonate_maintenance')),
 (SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname IN ('sathonate_networks','sathonate_network_revisions','sathonate_operations','sathonate_deployments','sathonate_maintenance') AND c.relrowsecurity AND c.relforcerowsecurity)
 FROM pg_roles r WHERE r.rolname=current_user`).Scan(&super, &bypass, &encrypted, &tables, &rls)
	if err != nil || super || bypass || !encrypted || tables != 5 || rls != 5 {
		result.Status = "BLOCKED"
		result.Detail = "Database TLS, restricted role or required schema/RLS prerequisites were not verified"
		return result
	}
	result.Status = "PASS"
	result.Detail = "Verified TLS connection, non-superuser/non-bypass role and required schema/RLS metadata observed; backup and application isolation remain separate checks"
	return result
}
func securePostgresConfig(connection string) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(connection)
	if err != nil {
		return nil, errors.New("invalid protected database configuration")
	}
	check := func(c *tls.Config) bool { return c != nil && !c.InsecureSkipVerify && c.ServerName != "" }
	if !check(cfg.ConnConfig.TLSConfig) {
		return nil, errors.New("verified TLS required")
	}
	for _, fallback := range cfg.ConnConfig.Fallbacks {
		if !check(fallback.TLSConfig) {
			return nil, errors.New("insecure database fallback forbidden")
		}
	}
	cfg.MaxConns = 1
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	return cfg, nil
}
func WithDatabaseEvidence(e ReadinessEvidence, check ReadinessCheck) ReadinessEvidence {
	if check.ID != "postgresql" {
		return e
	}
	found := false
	for i := range e.Checks {
		if e.Checks[i].ID == check.ID {
			e.Checks[i] = check
			found = true
		}
	}
	if !found {
		e.Checks = append(e.Checks, check)
	}
	if check.Status == "BLOCKED" {
		e.Status = "BLOCKED"
	}
	e.FinishedAt = time.Now().UTC()
	return e
}
