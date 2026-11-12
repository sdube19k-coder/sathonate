package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"sathonate.local/fabric-control-plane/internal/customer"
	"sathonate.local/fabric-control-plane/internal/identity"
	"sathonate.local/fabric-control-plane/internal/operations"
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

	_ = tx.Rollback(ctx)
	if runtime.MeterReady(ctx) == nil {
		t.Fatal("journal-writable meter accepted")
	}
	if _, err := admin.pool.Exec(ctx, "REVOKE INSERT,UPDATE,DELETE ON sathonate_operations,sathonate_deployments FROM fixture_api"); err != nil {
		t.Fatal(err)
	}
	defer admin.pool.Exec(ctx, "GRANT INSERT,UPDATE,DELETE ON sathonate_operations,sathonate_deployments TO fixture_api")
	if err := runtime.MeterReady(ctx); err != nil {
		t.Fatal("journal-reader meter rejected", err)
	}
	project := view.Projects[0].ID
	if err := svc.ApproveProject(ctx, "pilot-a", project, "fixture-approval"); err != nil {
		t.Fatal(err)
	}
	scope := (identity.Principal{TenantID: "pilot-a", ProjectID: project}).StorageTenant()
	finished := time.Now().Add(-time.Minute).UTC()
	for i, tag := range []string{"customer-kubernetes", "secure-local", "customer-kubernetes"} {
		id := "op_" + strings.Repeat(string(rune('a'+i)), 32)
		actualScope := scope
		if i == 2 {
			actualScope = "foreign-meter-scope"
		}
		record, _ := json.Marshal(operations.Operation{ID: id, TenantID: actualScope, Status: "succeeded", FinishedAt: &finished})
		result, _ := json.Marshal(map[string]any{"operationId": id, "mode": tag, "phase": "succeeded", "verified": true, "ready": true, "simulated": false, "transactionId": strings.Repeat("f", 64)})
		if _, err := admin.pool.Exec(ctx, "INSERT INTO sathonate_operations(tenant_id,id,network_id,status,payload) VALUES($1,$2,'fixture','succeeded',$3)", actualScope, id, record); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.pool.Exec(ctx, "INSERT INTO sathonate_deployments(tenant_id,operation_id,payload) VALUES($1,$2,$3)", actualScope, id, result); err != nil {
			t.Fatal(err)
		}
	}
	meter := customer.Meter{Service: svc, Source: runtime}
	for range 2 {
		receipt, err := meter.Sweep(ctx, "pilot-a", project, 24*time.Hour)
		if err != nil || receipt.Acknowledged != 1 {
			t.Fatalf("meter result %+v %v", receipt, err)
		}
	}
	after, err := runtime.Read(ctx, "pilot-a")
	if err != nil || len(after.Usage) != 1 || after.Usage[0].VerifiedOperations != 1 {
		t.Fatal("database meter double counted or dropped usage")
	}
}
