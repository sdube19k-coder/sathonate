// Package acceptance prepares/executes individually authorized qualification
// steps against an existing EKS target. It never provisions infrastructure.
package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sathonate.local/fabric-control-plane/internal/cloud"
	"sathonate.local/fabric-control-plane/internal/deployment"
	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/localfabric"
	"sathonate.local/fabric-control-plane/internal/maintenance"
	"sathonate.local/fabric-control-plane/internal/operations"
	"strconv"
	"strings"
	"time"
)

type Approval struct {
	Action           string    `json:"action"`
	TargetSHA256     string    `json:"targetSHA256"`
	NetworkSHA256    string    `json:"networkSHA256"`
	AuthorizationRef string    `json:"authorizationRef"`
	ExpiresAt        time.Time `json:"expiresAt"`
}
type Config struct {
	Registry                                                                                             cloud.Registry
	Target                                                                                               cloud.Target
	Network                                                                                              domain.NetworkSpec
	TenantID, NetworkID, Action, API, OperationID, IdempotencyKey, NodeID, ExpectedJobUID, CleanupDigest string
	AllowCloudRead, Execute                                                                              bool
	BudgetRef                                                                                            string
	Approvals                                                                                            []Approval
	Commander                                                                                            cloud.Commander
}
type Check struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}
type Evidence struct {
	Schema                     string     `json:"schema"`
	Action                     string     `json:"action"`
	At                         time.Time  `json:"at"`
	TargetSHA256               string     `json:"targetSHA256"`
	NetworkSHA256              string     `json:"networkSHA256"`
	OperationID                string     `json:"operationId,omitempty"`
	JobUID                     string     `json:"jobUID,omitempty"`
	TransactionID              string     `json:"transactionId,omitempty"`
	LedgerCheckedAt            *time.Time `json:"ledgerCheckedAt,omitempty"`
	CleanupDigest              string     `json:"cleanupDigest,omitempty"`
	Checks                     []Check    `json:"checks"`
	EKSQualified               bool       `json:"eksQualified"`
	ProductionExecutionEnabled bool       `json:"productionExecutionEnabled"`
}

var actions = map[string]bool{"plan": false, "preflight": false, "observe": false, "storage": false, "isolation": false, "adopt": false, "cleanup-preview": false, "deploy": true, "verify": true, "retry": true, "cancel": true, "disrupt": true, "cleanup": true}
var operationPattern = regexp.MustCompile(`^op_[a-f0-9]{32}$`)

func Validate(c Config, now time.Time) (cloud.OperatorPlan, error) {
	mutation, ok := actions[c.Action]
	if !ok {
		return cloud.OperatorPlan{}, errors.New("unsupported acceptance action")
	}
	registered, ok := c.Registry.Find(c.TenantID, c.Target.ID)
	if !ok || cloud.TargetDigest(registered) != cloud.TargetDigest(c.Target) {
		return cloud.OperatorPlan{}, errors.New("acceptance target not registered for this tenant")
	}
	plan, err := cloud.CompileOperator(c.Network, c.Target, deployment.Namespace(c.TenantID, c.NetworkID))
	if err != nil {
		return cloud.OperatorPlan{}, errors.New("acceptance specification does not match exact approved target")
	}
	if c.Action != "plan" && !c.AllowCloudRead {
		return plan, errors.New("explicit cloud-read opt-in required")
	}
	if mutation {
		if !c.Execute || !regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`).MatchString(c.BudgetRef) {
			return plan, errors.New("explicit execution and budget authorization required")
		}
		approved := false
		for _, a := range c.Approvals {
			approved = approved || (a.Action == c.Action && a.TargetSHA256 == cloud.TargetDigest(c.Target) && a.NetworkSHA256 == plan.NetworkSHA256 && regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`).MatchString(a.AuthorizationRef) && a.ExpiresAt.After(now) && !a.ExpiresAt.After(now.Add(time.Hour)))
		}
		if !approved {
			return plan, errors.New("current exact per-action administrator approval required")
		}
	}
	return plan, nil
}
func client(base string) (*http.Client, string, error) {
	u, e := url.Parse(base)
	if e != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "http" && u.Scheme != "https") || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback() {
		return nil, "", errors.New("qualification API must be an explicit loopback origin without credentials")
	}
	return &http.Client{Timeout: 25 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, strings.TrimRight(u.String(), "/"), nil
}
func request(ctx context.Context, h *http.Client, base, method, path string, body any, headers map[string]string, out any) error {
	var input io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		input = bytes.NewReader(raw)
	}
	r, e := http.NewRequestWithContext(ctx, method, base+"/api/v1"+path, input)
	if e != nil {
		return errors.New("invalid qualification API request")
	}
	r.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	response, e := h.Do(r)
	if e != nil {
		return errors.New("qualification API unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errors.New("qualification API rejected the bound action; response body withheld")
	}
	if out == nil {
		return nil
	}
	if json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(out) != nil {
		return errors.New("invalid qualification API response")
	}
	return nil
}
func Run(ctx context.Context, c Config) (Evidence, error) {
	e := Evidence{Schema: "sathonate.eks-acceptance/v1", Action: c.Action, At: time.Now().UTC(), Checks: []Check{}}
	add := func(id, status string) { e.Checks = append(e.Checks, Check{id, status}) }
	plan, err := Validate(c, e.At)
	if err != nil {
		add("authorization-and-binding", "BLOCKED")
		return e, err
	}
	e.TargetSHA256 = cloud.TargetDigest(c.Target)
	e.NetworkSHA256 = plan.NetworkSHA256
	for _, id := range []string{"ca-tls-enrollment", "peer-and-raft-health", "channel-transaction", "ebs-volume-attachment", "disruption-recovery", "controller-restart", "duplicate-retry-cancel", "tenant-isolation", "cleanup-retention", "backup-restore", "certificate-lifecycle"} {
		add(id, "NOT VERIFIED")
	}
	if c.Action == "plan" {
		add("offline-binding", "PASS")
		return e, nil
	}
	commands := c.Commander
	if commands == nil {
		commands = cloud.OSCommander{}
	}
	if c.Action == "preflight" || c.Action == "deploy" || c.Action == "verify" || c.Action == "disrupt" {
		err = cloud.OperatorPreflight(ctx, c.Target, commands)
	} else {
		err = cloud.OperatorAccess(ctx, c.Target, commands)
	}
	if err != nil {
		add("live-cluster-preflight", "BLOCKED")
		return e, errors.New("approved live cluster access/preflight failed")
	}
	add("live-cluster-access", "PASS")
	if c.Action == "preflight" {
		return e, nil
	}
	h, base, err := client(c.API)
	if err != nil {
		return e, err
	}
	var meta struct {
		Mode, DeploymentMode string
		ExecutionEnabled     bool
	}
	if request(ctx, h, base, "GET", "/meta", nil, nil, &meta) != nil || meta.Mode != "local-development" || meta.DeploymentMode != "aws-eks" || meta.ExecutionEnabled {
		return e, errors.New("API is not the gated loopback EKS qualification executor")
	}
	var draft struct {
		ID       string
		Revision int
		Desired  domain.NetworkSpec
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`).MatchString(c.NetworkID) {
		return e, errors.New("invalid saved network reference")
	}
	if request(ctx, h, base, "GET", "/networks/"+c.NetworkID, nil, nil, &draft) != nil || draft.ID != c.NetworkID || maintenance.HashBytes(mustJSON(draft.Desired)) != plan.NetworkSHA256 {
		return e, errors.New("API saved network differs from approved specification")
	}
	kub := func(args ...string) ([]byte, error) {
		return commands.Run(ctx, "kubectl", append([]string{"--kubeconfig", c.Target.Operator.Kubeconfig, "--context", c.Target.KubeContext, "--request-timeout=20s"}, args...))
	}
	ns := c.Target.RunnerNamespace
	if c.Action == "deploy" || c.Action == "verify" {
		if !operations.ValidKey(c.IdempotencyKey) {
			return e, errors.New("explicit idempotency key required")
		}
		if c.Action == "deploy" {
			raw, er := kub("-n", ns, "get", "pods,persistentvolumeclaims,fabriccas,fabricpeers,fabricorderernodes,fabricchaincodes", "-o", "jsonpath={.items[*].metadata.uid}")
			if er != nil || len(bytes.TrimSpace(raw)) != 0 {
				return e, errors.New("fresh deployment requires an empty approved namespace")
			}
		}
		headers := map[string]string{"If-Match": strconv.Quote(strconv.Itoa(draft.Revision)), "Idempotency-Key": c.IdempotencyKey}
		var first, second struct{ OperationID string }
		if request(ctx, h, base, "POST", "/networks/"+c.NetworkID+"/deploy", map[string]any{}, headers, &first) != nil || !operationPattern.MatchString(first.OperationID) {
			return e, errors.New("deployment submission failed")
		}
		e.OperationID = first.OperationID
		if request(ctx, h, base, "POST", "/networks/"+c.NetworkID+"/deploy", map[string]any{}, headers, &second) != nil || second.OperationID != first.OperationID {
			return e, errors.New("duplicate submission did not adopt same operation")
		}
		add("duplicate-submission", "PASS")
		add("deployment-submitted-not-yet-verified", "PASS")
		return e, nil
	}
	if !operationPattern.MatchString(c.OperationID) {
		return e, errors.New("explicit valid operation reference required")
	}
	e.OperationID = c.OperationID
	var op operations.Operation
	var deployed deployment.Result
	if request(ctx, h, base, "GET", "/operations/"+c.OperationID, nil, nil, &op) != nil || op.NetworkID != c.NetworkID {
		return e, errors.New("operation does not belong to selected saved network")
	}
	if request(ctx, h, base, "GET", "/deployments/"+c.OperationID, nil, nil, &deployed) != nil || deployed.Simulated || deployed.Mode != "aws-eks" || deployed.TargetSHA256 != e.TargetSHA256 || maintenance.HashBytes(mustJSON(deployed.Network)) != plan.NetworkSHA256 {
		return e, errors.New("deployment binding or simulation mismatch")
	}
	switch c.Action {
	case "observe", "adopt":
		var health localfabric.Health
		if request(ctx, h, base, "GET", "/deployments/"+c.OperationID+"/health", nil, nil, &health) != nil {
			return e, errors.New("current workload health unavailable")
		}
		if health.Ready {
			add("current-workload-health", "PASS")
		}
		if op.Status == "succeeded" && deployed.Verified && regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(deployed.TransactionID) && len(health.Peers) == 2 && health.Peers[0].Height > 0 && health.Peers[0].Height == health.Peers[1].Height && health.Peers[0].Hash == health.Peers[1].Hash {
			e.TransactionID = deployed.TransactionID
			e.LedgerCheckedAt = &health.CheckedAt
			add("recorded-transaction-proof", "PASS")
		}
		raw, er := kub("-n", ns, "get", "job", "smp-"+strings.TrimPrefix(c.OperationID, "op_"), "-o", "jsonpath={.metadata.uid}")
		if er != nil {
			return e, errors.New("worker Job identity unavailable")
		}
		uid := strings.TrimSpace(string(raw))
		if !regexp.MustCompile(`^[a-f0-9-]{36}$`).MatchString(uid) {
			return e, errors.New("invalid worker Job UID")
		}
		e.JobUID = uid
		if c.Action == "adopt" {
			if c.ExpectedJobUID == "" || uid != c.ExpectedJobUID {
				return e, errors.New("Job was replaced or expected UID not supplied")
			}
			add("job-uid-stability-after-operator-managed-restart", "PASS")
		}
	case "storage":
		raw, er := kub("-n", ns, "get", "persistentvolumeclaims", "-o", "json")
		var claims struct {
			Items []struct {
				Metadata struct{ UID string }
				Spec     struct{ VolumeName, StorageClassName string }
				Status   struct{ Phase string }
			}
		}
		if er != nil || json.Unmarshal(raw, &claims) != nil || len(claims.Items) < 7 {
			return e, errors.New("expected bound Fabric PVC inventory unavailable")
		}
		for _, claim := range claims.Items {
			if claim.Status.Phase != "Bound" || claim.Spec.StorageClassName != c.Target.StorageClass || !regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,252}$`).MatchString(claim.Spec.VolumeName) {
				return e, errors.New("Fabric claim storage binding invalid")
			}
			raw, er = kub("get", "persistentvolume", claim.Spec.VolumeName, "-o", "json")
			var volume struct {
				Spec struct {
					CSI      struct{ Driver, VolumeHandle string }
					ClaimRef struct{ UID, Namespace string }
				}
			}
			if er != nil || json.Unmarshal(raw, &volume) != nil || volume.Spec.CSI.Driver != "ebs.csi.aws.com" || volume.Spec.ClaimRef.UID != claim.Metadata.UID || volume.Spec.ClaimRef.Namespace != ns || !strings.HasPrefix(volume.Spec.CSI.VolumeHandle, "vol-") {
				return e, errors.New("actual EBS CSI volume binding not established")
			}
		}
		add("bound-ebs-csi-volume-references", "PASS")
		add("actual-ebs-encryption-and-snapshot-recovery", "NOT VERIFIED")
	case "isolation":
		actor := "--as=system:serviceaccount:" + ns + ":smp-worker"
		raw, _ := kub("auth", "can-i", actor, "get", "pods", "-n", ns)
		if strings.TrimSpace(string(raw)) != "yes" {
			return e, errors.New("worker positive RBAC control or impersonation permission unavailable")
		}
		for _, args := range [][]string{{"get", "secrets", "-n", "default"}, {"create", "fabricmainchannels.hlf.kungfusoftware.es"}, {"list", "fabricmainchannels.hlf.kungfusoftware.es"}} {
			raw, _ = kub(append([]string{"auth", "can-i", actor}, args...)...)
			if strings.TrimSpace(string(raw)) != "no" {
				return e, errors.New("worker negative RBAC control not established")
			}
		}
		add("worker-positive-and-negative-rbac", "PASS")
		add("live-okta-and-cni-tenant-denial", "NOT VERIFIED")
	case "cancel":
		if request(ctx, h, base, "POST", "/deployments/"+c.OperationID+"/cancel", map[string]any{}, nil, nil) != nil {
			return e, errors.New("durable cancellation request failed")
		}
		add("cancellation-accepted", "PASS")
	case "retry":
		if !operations.ValidKey(c.IdempotencyKey) {
			return e, errors.New("explicit retry key required")
		}
		var retried struct{ OperationID string }
		if request(ctx, h, base, "POST", "/operations/"+c.OperationID+"/retry", map[string]any{}, map[string]string{"Idempotency-Key": c.IdempotencyKey}, &retried) != nil || !operationPattern.MatchString(retried.OperationID) {
			return e, errors.New("bound retry submission failed")
		}
		e.OperationID = retried.OperationID
		add("retry-submitted", "PASS")
	case "disrupt":
		approved := false
		for _, node := range c.Network.Spec.Nodes {
			approved = approved || node.ID == c.NodeID
		}
		if !approved {
			return e, errors.New("disruption is limited to one approved Fabric node")
		}
		if _, er := kub("-n", ns, "rollout", "restart", "deployment/"+c.NodeID); er != nil {
			return e, errors.New("controlled single-node restart failed")
		}
		add("single-node-restart-requested", "PASS")
		add("post-disruption-fresh-transaction", "NOT VERIFIED")
	case "cleanup-preview":
		var preview deployment.CleanupPreview
		if request(ctx, h, base, "GET", "/deployments/"+c.OperationID+"/cleanup-preview", nil, nil, &preview) != nil || preview.TargetSHA256 != e.TargetSHA256 || preview.NetworkSHA256 != e.NetworkSHA256 {
			return e, errors.New("bound cleanup preview unavailable")
		}
		e.CleanupDigest = preview.Digest
		add("exact-cleanup-preview", "PASS")
	case "cleanup":
		if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(c.CleanupDigest) {
			return e, errors.New("explicit reviewed cleanup digest required")
		}
		if request(ctx, h, base, "DELETE", "/deployments/"+c.OperationID, nil, map[string]string{"X-Sathonate-Cleanup-Approval": c.CleanupDigest}, nil) != nil {
			return e, errors.New("approved precise cleanup failed")
		}
		add("workload-and-worker-access-cleanup", "PASS")
		add("retained-data-and-cloud-cost-verification", "NOT VERIFIED")
	}
	return e, nil
}
func mustJSON(v any) []byte { raw, _ := json.Marshal(v); return raw }

// Fixed failure wrapper for CLI callers; never return API response payloads.
func FailureMessage(action string) string {
	if _, ok := actions[action]; !ok {
		return "Acceptance action blocked"
	}
	return fmt.Sprintf("Acceptance %s blocked or incomplete; no raw diagnostics exported", action)
}
