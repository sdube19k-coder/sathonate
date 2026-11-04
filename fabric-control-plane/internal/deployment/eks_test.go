package deployment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"sathonate.local/fabric-control-plane/internal/cloud"
	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/operations"
)

func TestEKSDeniesUnregisteredTenantAndMissingApproval(t *testing.T) {
	s := fixture(t)
	r := EKSRunner{}
	if r.Validate(Request{TenantID: "other", NetworkID: "network", Network: s}) == nil {
		t.Fatal("unregistered target accepted")
	}
	r.Registry.Targets = []cloud.Target{{ID: s.Spec.Deployment.TargetRef, TenantID: "test-a"}}
	if r.Validate(Request{TenantID: "test-a", NetworkID: "network", Network: s}) == nil {
		t.Fatal("missing approval accepted")
	}
}

func TestEKSApprovedBindingCannotCrossTenantOrDrift(t *testing.T) {
	raw, e := os.ReadFile("../../examples/eks-qualification-network.json")
	if e != nil {
		t.Fatal(e)
	}
	var s domain.NetworkSpec
	if json.Unmarshal(raw, &s) != nil {
		t.Fatal("fixture")
	}
	raw, e = os.ReadFile("../../examples/eks-operator-targets.template.json")
	if e != nil {
		t.Fatal(e)
	}
	var registry cloud.Registry
	if json.Unmarshal(raw, &registry) != nil {
		t.Fatal("registry fixture")
	}
	target := registry.Targets[0]
	target.TenantID = "test-a"
	target.RunnerNamespace = Namespace("test-a", "network")
	raw, _ = json.Marshal(s)
	hash := sha256.Sum256(raw)
	target.Operator.NetworkSHA256 = hex.EncodeToString(hash[:])
	registry.Targets[0] = target
	runner := EKSRunner{Registry: registry, Approvals: []cloud.Approval{{Purpose: "eks-qualification", TargetDigest: cloud.TargetDigest(target), AuthorizationRef: "change-123", ExpiresAt: time.Now().Add(time.Hour)}}}
	request := Request{TenantID: "test-a", NetworkID: "network", Network: s, TargetSHA256: cloud.TargetDigest(target)}
	if e = runner.Validate(request); e != nil {
		t.Fatal(e)
	}
	request.TenantID = "test-b"
	if runner.Validate(request) == nil {
		t.Fatal("foreign tenant accepted")
	}
	request.TenantID = "test-a"
	request.TargetSHA256 = strings.Repeat("f", 64)
	if runner.Validate(request) == nil {
		t.Fatal("target drift accepted")
	}
	request.TargetSHA256 = cloud.TargetDigest(target)
	request.Network.Spec.Organizations[0].MSPID = "ChangedMSP"
	if runner.Validate(request) == nil {
		t.Fatal("unapproved network digest accepted")
	}
}
func TestEKSManifestHasNoClusterAdminOrInlineCloudCredentials(t *testing.T) {
	request := Request{TenantID: "tenant-a", NetworkID: "network", Network: fixture(t)}
	name := "smp-" + strings.Repeat("a", 32)
	runner := KubernetesRunner{eks: &eksBinding{target: cloud.Target{RunnerNamespace: Namespace(request.TenantID, request.NetworkID), RunnerImage: "registry/worker@sha256:" + strings.Repeat("a", 64), Operator: &cloud.OperatorTarget{}}}}
	manifest, err := runner.eksManifest(request, name)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range manifest["items"].([]any) {
		object := item.(map[string]any)
		if object["kind"] == "ClusterRole" {
			for _, rule := range object["rules"].([]any) {
				r := rule.(map[string]any)
				if len(r["resourceNames"].([]string)) != 3 {
					t.Fatal("unbounded channel reads")
				}
				for _, verb := range r["verbs"].([]string) {
					if verb != "get" && verb != "list" && verb != "watch" {
						t.Fatal("worker can mutate cluster-scoped resources")
					}
				}
			}
		}
		if object["kind"] == "ClusterRoleBinding" && object["roleRef"].(map[string]any)["name"] == "cluster-admin" {
			t.Fatal("cluster-admin worker")
		}
	}
}

func TestChannelReplayAdoptsDefaultsButRejectsDrift(t *testing.T) {
	desired := map[string]any{"name": "sathonate", "organizations": []any{map[string]any{"namespace": "tenant-a"}}, "external": []any{}}
	actual := map[string]any{"name": "sathonate", "organizations": []any{map[string]any{"namespace": "tenant-a", "defaultField": true}}, "external": nil, "serverDefault": true}
	if !desiredFieldsMatch(actual, desired) {
		t.Fatal("server defaults prevented safe adoption")
	}
	actual["organizations"] = []any{map[string]any{"namespace": "tenant-b"}}
	if desiredFieldsMatch(actual, desired) {
		t.Fatal("foreign namespace drift adopted")
	}
}

type recoveryRunner struct {
	started   chan string
	block     bool
	cancelled chan string
}

func (*recoveryRunner) Mode() string { return "aws-eks" }
func (r *recoveryRunner) Run(ctx context.Context, request Request, emit func(string, string) error) (Outcome, error) {
	if e := emit("validate", "Observed test worker start"); e != nil {
		return Outcome{}, e
	}
	r.started <- request.OperationID
	if r.block {
		<-ctx.Done()
		return Outcome{}, ctx.Err()
	}
	return Outcome{}, errors.New("simulated upstream failure: password=do-not-expose")
}
func (r *recoveryRunner) CancelDeployment(_ context.Context, request Request) error {
	if r.cancelled != nil {
		r.cancelled <- request.OperationID
	}
	return nil
}
func waitStarted(t *testing.T, c chan string, id string) {
	t.Helper()
	select {
	case got := <-c:
		if got != id {
			t.Fatal("recovery replaced operation", got, id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("durable operation did not dispatch")
	}
}

func TestEKSPostgresRecoveryRetryCancellation(t *testing.T) {
	url := os.Getenv("SATHONATE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires dedicated PostgreSQL test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	pool, e := pgxpool.New(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	if _, e = pool.Exec(ctx, operations.SQLSchema); e != nil {
		t.Fatal(e)
	}
	key := fmt.Sprintf("eks-%d", time.Now().UnixNano())
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM sathonate_deployments WHERE operation_id IN (SELECT id FROM sathonate_operations WHERE tenant_id='test-a' AND network_id=$1)", key)
		_, _ = pool.Exec(context.Background(), "DELETE FROM sathonate_operations WHERE tenant_id='test-a' AND network_id=$1", key)
	}()
	first := &recoveryRunner{started: make(chan string, 4), block: true}
	m, e := OpenDatabase(ctx, pool, first)
	if e != nil {
		t.Fatal(e)
	}
	op, e := m.Start("test-a", "operator", key, fixture(t), 1, key, "", "")
	if e != nil {
		m.Close()
		t.Fatal(e)
	}
	waitStarted(t, first.started, op.ID)
	if other, err := OpenDatabase(ctx, pool, first); err == nil {
		other.Close()
		m.Close()
		t.Fatal("two schedulers own the database")
	}
	// Terminate only this test manager's own lock session, proving that it
	// stops dispatching after losing its fence. Never target another service.
	if _, e = pool.Exec(ctx, "SELECT pg_terminate_backend($1)", int32(m.lease.Conn().PgConn().PID())); e != nil {
		m.Close()
		t.Fatal(e)
	}
	select {
	case <-m.ctx.Done():
	case <-time.After(5 * time.Second):
		m.Close()
		t.Fatal("lock-session loss did not stop executor")
	}
	if _, e = m.Start("test-a", "operator", key, fixture(t), 1, key+"-lost-lock", "", ""); e == nil {
		m.Close()
		t.Fatal("executor accepted work after lock loss")
	}
	m.Close()
	second := &recoveryRunner{started: make(chan string, 4), cancelled: make(chan string, 4)}
	m, e = OpenDatabase(ctx, pool, second)
	if e != nil {
		t.Fatal(e)
	}
	waitStarted(t, second.started, op.ID)
	result := terminal(t, m, op.ID)
	if result.Status != "failed" || strings.Contains(result.Error, "do-not-expose") {
		m.Close()
		t.Fatal("failure/redaction incorrect", result.Error)
	}
	same, e := m.Start("test-a", "operator", key, fixture(t), 1, key, "", "")
	if e != nil || same.ID != op.ID {
		m.Close()
		t.Fatal("idempotency lost on recovery", e)
	}
	retry, e := m.Retry("test-a", "operator", op.ID, key+"-retry")
	if e != nil {
		m.Close()
		t.Fatal(e)
	}
	waitStarted(t, second.started, retry.ID)
	if terminal(t, m, retry.ID).Status != "failed" {
		m.Close()
		t.Fatal("unexpected retry result")
	}
	m.Close()
	third := &recoveryRunner{started: make(chan string, 4), cancelled: make(chan string, 4), block: true}
	m, e = OpenDatabase(ctx, pool, third)
	if e != nil {
		t.Fatal(e)
	}
	retry, e = m.Retry("test-a", "operator", retry.ID, key+"-cancel")
	if e != nil {
		m.Close()
		t.Fatal(e)
	}
	waitStarted(t, third.started, retry.ID)
	if e = m.Cancel(ctx, "other-tenant", retry.ID); e == nil {
		m.Close()
		t.Fatal("cross-tenant cancellation accepted")
	}
	if e = m.Cancel(ctx, "test-a", retry.ID); e != nil {
		m.Close()
		t.Fatal(e)
	}
	if terminal(t, m, retry.ID).Status != "cancelled" {
		m.Close()
		t.Fatal("cancellation not persisted")
	}
	m.Close()
	m, e = OpenDatabase(ctx, pool, third)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	if terminal(t, m, retry.ID).Status != "cancelled" {
		t.Fatal("cancelled operation resurrected")
	}
}
