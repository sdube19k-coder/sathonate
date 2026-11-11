package persistence

import (
	"context"
	"errors"
	"sync"
	"testing"

	"sathonate.local/fabric-control-plane/internal/customer"
)

func customerPostgresFixture(t *testing.T, ctx context.Context, admin, service *Database) {
	t.Helper()
	for _, statement := range []string{
		"CREATE ROLE sathonate_customer_admin NOLOGIN",
		"GRANT sathonate_customer_admin TO fixture_admin",
		"GRANT SELECT,UPDATE(payload) ON public.sathonate_customer_accounts TO fixture_api",
		"GRANT SELECT,INSERT ON public.sathonate_customer_changes TO fixture_api",
	} {
		if _, err := admin.pool.Exec(ctx, statement); err != nil {
			t.Fatal("customer role fixture setup", err)
		}
	}
	bootstrap := customer.Postgres{Pool: admin.pool}
	runtime := customer.Postgres{Pool: service.pool}
	if err := runtime.Ready(ctx); err != nil {
		t.Fatal("restricted customer role not ready", err)
	}
	const issuer = "https://id.example.test/oauth2/default"
	for _, tenant := range []string{"pilot-a", "pilot-b"} {
		if err := bootstrap.Bootstrap(ctx, tenant, tenant, customer.Identity{Issuer: issuer, Subject: tenant}, ""); err != nil {
			t.Fatal(err)
		}
	}
	actor := customer.Actor{Issuer: issuer, Subject: "pilot-a", Tenant: "pilot-a", Project: "existing"}
	svc := customer.Service{Store: runtime}
	var group sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			results <- svc.Execute(ctx, actor, "start-trial-test-001", customer.Command{Action: "start-trial"})
		}()
	}
	group.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal("concurrent replay", err)
		}
	}
	if err := svc.Execute(ctx, actor, "request-project-001", customer.Command{Action: "request-project", Name: "Pilot project"}); err != nil {
		t.Fatal(err)
	}
	view, err := svc.View(ctx, actor)
	if err != nil || !view.ActiveEntitlement || len(view.Projects) != 1 {
		t.Fatal("persisted trial/project missing")
	}
	actor.Tenant = "pilot-b"
	if _, err := svc.View(ctx, actor); !errors.Is(err, customer.ErrDenied) {
		t.Fatal("foreign account visible")
	}
	tx, err := service.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT set_config('sathonate.customer_tenant','pilot-a',true)"); err != nil {
		t.Fatal(err)
	}
	var accounts, changes int
	if err = tx.QueryRow(ctx, "SELECT (SELECT count(*) FROM sathonate_customer_accounts),(SELECT count(*) FROM sathonate_customer_changes)").Scan(&accounts, &changes); err != nil || accounts != 1 || changes != 3 {
		t.Fatalf("unexpected scoped counts %d/%d: %v", accounts, changes, err)
	}
	if _, err = tx.Exec(ctx, "UPDATE sathonate_customer_changes SET action='tampered'"); err == nil {
		t.Fatal("runtime can edit audit")
	}
	if err := runtime.Bootstrap(ctx, "pilot-c", "Foreign", customer.Identity{Issuer: issuer, Subject: "someone"}, ""); !errors.Is(err, customer.ErrDenied) {
		t.Fatal("runtime can bootstrap accounts")
	}
}
