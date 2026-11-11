package customer

import (
	"context"
	"strings"
	"testing"
	"time"

	"sathonate.local/fabric-control-plane/internal/identity"
)

func TestMeterRejectsUnqualifiedOperations(t *testing.T) {
	ctx := context.Background()
	tenant, project := "tenant", "project"
	store := &fixtureStore{map[string]Account{tenant: {Projects: []Project{{ID: project, State: "active"}}}}, map[string]Change{}}
	svc := Service{Store: store}
	good := VerifiedOperation{ID: "op_" + strings.Repeat("a", 32), Scope: (identity.Principal{TenantID: tenant, ProjectID: project}).StorageTenant(),
		Mode: "customer-kubernetes", Status: "succeeded", TransactionID: strings.Repeat("b", 64), Ready: true, Verified: true, FinishedAt: time.Now()}
	for _, mutate := range []func(*VerifiedOperation){
		func(r *VerifiedOperation) { r.Simulated = true },
		func(r *VerifiedOperation) { r.Mode = "secure-local" },
		func(r *VerifiedOperation) { r.Status = "failed" },
		func(r *VerifiedOperation) { r.Verified = false },
		func(r *VerifiedOperation) { r.Scope = "foreign" },
		func(r *VerifiedOperation) { r.TransactionID = "" },
	} {
		rejected := good
		mutate(&rejected)
		if err := svc.RecordVerifiedOperation(ctx, tenant, project, rejected); err != ErrDenied {
			t.Fatal("ineligible operation accepted")
		}
	}
	for range 2 {
		if err := svc.RecordVerifiedOperation(ctx, tenant, project, good); err != nil {
			t.Fatal(err)
		}
	}
	if store.accounts[tenant].Usage[0].VerifiedOperations != 1 || len(store.audit) != 1 {
		t.Fatal("double meter entry")
	}
}
