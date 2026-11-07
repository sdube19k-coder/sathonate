package deployment

import (
	"context"
	"errors"
	"path/filepath"
	"sathonate.local/fabric-control-plane/internal/operations"
	"strings"
	"testing"
)

type deletionFixtureRunner struct {
	MockRunner
	plan     CleanupPreview
	fail     bool
	calls    *int
	observed func() bool
}

func (deletionFixtureRunner) Mode() string { return "secure-local" }
func (r deletionFixtureRunner) PreviewCleanup(context.Context, Request) (CleanupPreview, error) {
	return r.plan, nil
}
func (r deletionFixtureRunner) deleteRetainingData(context.Context, Request, CleanupPreview) error {
	*r.calls++
	if !r.observed() {
		return errors.New("intent was not durable before deletion")
	}
	if r.fail {
		return errors.New("injected deletion interruption")
	}
	return nil
}
func TestRetainedDeletionApprovalDurabilityReplayAndActorIsolation(t *testing.T) {
	root := t.TempDir()
	jobs, e := operations.Open(filepath.Join(root, "ops"))
	if e != nil {
		t.Fatal(e)
	}
	op, _, e := jobs.Begin("tenant", "operator", "network", "deploy", "base-key", "digest", "version", "", []operations.Stage{{Key: "verify"}})
	if e != nil {
		t.Fatal(e)
	}
	jobs.Advance(op.ID, "verify")
	jobs.Finish(op.ID, nil)
	plan := CleanupPreview{Schema: "sathonate.local-retained-deletion/v1", NamespaceUID: "owned", TargetSHA256: strings.Repeat("a", 64), Delete: []CleanupRef{}, RetainedObjects: []RetainedRef{}}
	plan.Digest = cleanupDigest(plan)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	m := &Manager{root: root, jobs: jobs, ctx: ctx, cancel: cancel}
	observed := func() bool {
		s, e := m.Status("tenant", op.ID)
		return e == nil && s.LocalDeletion != nil && s.LocalDeletion.Plan.Digest == plan.Digest && s.Phase == "retiring"
	}
	m.runner = deletionFixtureRunner{plan: plan, fail: true, calls: &calls, observed: observed}
	if m.write(Result{TenantID: "tenant", OperationID: op.ID, Network: dynamicDesign(t), Mode: "secure-local", Phase: "running", Verified: true, Ready: true}) != nil {
		t.Fatal("fixture write")
	}
	if m.Cleanup(ctx, "tenant", op.ID) == nil {
		t.Fatal("implicit local cleanup bypassed approval")
	}
	if m.CleanupApproved(ctx, "foreign", op.ID, "admin", plan.Digest) == nil {
		t.Fatal("foreign tenant cleanup")
	}
	if m.CleanupApproved(ctx, "tenant", op.ID, "admin", strings.Repeat("f", 64)) == nil || calls != 0 {
		t.Fatal("stale approval reached mutation")
	}
	if m.CleanupApproved(ctx, "tenant", op.ID, "admin", plan.Digest) == nil || calls != 1 {
		t.Fatal("interrupted cleanup not recorded")
	}
	// Reopen the durable source store, then continue the same approved intent.
	jobs, e = operations.Open(filepath.Join(root, "ops"))
	if e != nil {
		t.Fatal(e)
	}
	m.jobs = jobs
	m.runner = deletionFixtureRunner{plan: plan, calls: &calls, observed: observed}
	if m.CleanupApproved(ctx, "tenant", op.ID, "other-admin", plan.Digest) == nil || calls != 1 {
		t.Fatal("actor substitution adopted deletion")
	}
	if m.CleanupApproved(ctx, "tenant", op.ID, "admin", plan.Digest) != nil || calls != 2 {
		t.Fatal("approved deletion did not recover")
	}
	after, e := m.Status("tenant", op.ID)
	if e != nil || after.Phase != "removed" || !after.LocalDeletion.Completed || !after.LocalDeletion.RetentionVerified || after.LocalDeletion.Plan.Digest != cleanupDigest(after.LocalDeletion.Plan) {
		t.Fatal("completion or immutable approval lost")
	}
	if m.CleanupApproved(ctx, "tenant", op.ID, "admin", plan.Digest) != nil || calls != 2 {
		t.Fatal("acknowledgement repeated deletion")
	}
	if _, e = m.Start("tenant", "admin", "network", dynamicDesign(t), 1, "redeploy-retired", "", ""); e == nil {
		t.Fatal("retired network redeployed")
	}
}
