package deployment

// Offline export for an administrator's isolated qualification. This does not
// implement Runner, select an AWS API route, or relax the kind-only runner guard.
import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"

	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/kubernetesfabric"
)

type QualificationExport struct {
	TenantID, NetworkID, OperationID string
	WorkerImage, ControllerImage     string
	APIIPs                           []string
	Network                          domain.NetworkSpec
	Runtime                          kubernetesfabric.Runtime
}

// ExportQualification preserves the already qualified protected issuer protocol.
// The input remains an explicit compatibility fixture, not an AWS network design.
// Identity/project authorization and an EKS execution adapter are NOT qualified
// by this export. All output must be bound to an approved cluster before applying.
func ExportQualification(input QualificationExport) (map[string]any, error) {
	denied := errors.New("invalid standalone secure qualification binding")
	if !regexp.MustCompile(`^lp-[a-f0-9]{48}$`).MatchString(input.TenantID) ||
		!regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(input.NetworkID) ||
		!regexp.MustCompile(`^op_[a-f0-9]{32}$`).MatchString(input.OperationID) ||
		!regexp.MustCompile(`^[a-zA-Z0-9./:_-]+@sha256:[a-f0-9]{64}$`).MatchString(input.WorkerImage) {
		return nil, denied
	}
	// Only a registry alias of the immutable qualified controller is permitted.
	controller := SecureLocalProfile().ControllerImage
	digest := controller[strings.LastIndex(controller, "@"):]
	if !regexp.MustCompile(`^[a-zA-Z0-9./:_-]+@sha256:[a-f0-9]{64}$`).MatchString(input.ControllerImage) || !strings.HasSuffix(input.ControllerImage, digest) {
		return nil, denied
	}
	if domain.HasErrors(domain.Validate(input.Network)) || input.Network.Spec.Deployment.TargetRef != domain.SecureLocalProfile || input.Runtime.Validate(input.Network) != nil || len(input.APIIPs) == 0 || len(input.APIIPs) > 16 {
		return nil, denied
	}
	for _, ip := range input.APIIPs {
		if net.ParseIP(ip) == nil || net.ParseIP(ip).IsUnspecified() || net.ParseIP(ip).IsLoopback() {
			return nil, denied
		}
	}
	// Three distinct exact zone placements are mandatory for the full Fabric run.
	zones := map[string]bool{}
	for _, node := range input.Network.Spec.Nodes {
		placement := input.Runtime.NodeSelectors[node.ID]
		if len(placement) != 1 || !regexp.MustCompile(`^ap-south-1[a-z]$`).MatchString(placement["topology.kubernetes.io/zone"]) {
			return nil, denied
		}
		if node.Role == "orderer" {
			zones[placement["topology.kubernetes.io/zone"]] = true
		}
	}
	if len(zones) != 3 || input.Runtime.Capacity != "10Gi" {
		return nil, denied
	}
	ns, err := SecureNamespace(input.TenantID, "token-bound-project", input.NetworkID)
	if err != nil {
		return nil, denied
	}
	r := SecureRunner{binding: &secureBinding{config: SecureLocalConfig{WorkerImage: input.WorkerImage, Runtime: input.Runtime}}}
	name, _ := jobName(input.OperationID)
	workloads, err := r.manifest(Request{TenantID: input.TenantID, NetworkID: input.NetworkID, OperationID: input.OperationID, Network: input.Network}, name, ns)
	if err != nil {
		return nil, denied
	}
	portable, err := portableQualificationWorkloads(workloads, input.ControllerImage)
	if err != nil {
		return nil, denied
	}
	isolation := isolationManifest(ns, input.APIIPs)
	// Seven 10Gi Fabric claims plus one claim of explicit storage headroom.
	for _, item := range isolation["items"].([]any) {
		obj := item.(map[string]any)
		if obj["kind"] == "ResourceQuota" {
			obj["spec"].(map[string]any)["hard"].(map[string]any)["requests.storage"] = "80Gi"
		}
	}
	labels := map[string]string{"fabric.sathonate.io/tenant": input.TenantID, "fabric.sathonate.io/project": "token-bound-project", "fabric.sathonate.io/network": input.NetworkID, "fabric.sathonate.io/profile": domain.SecureLocalProfile, "pod-security.kubernetes.io/enforce": "baseline", "pod-security.kubernetes.io/enforce-version": "v1.34"}
	return map[string]any{
		"schema": "sathonate.secure-qualification-export/v1", "namespace": ns, "job": name,
		"notice":            "Standalone protected-issuer compatibility fixture. Not API-to-EKS, production, signing/Raft rotation or Sprint 2 incident qualification.",
		"productionEnabled": false, "awsExecuted": false,
		"namespaceManifest": map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": ns, "labels": labels}},
		"isolationManifest": isolation, "workloadManifest": portable,
		"applyOrder":                     []string{"CREATE fresh namespace; fail if occupied", "APPLY isolation; verify enforcing CNI and positive/negative connectivity before secrets", "APPLY reviewed workloads"},
		"requiredControllerBinarySHA256": SecureLocalProfile().ControllerBinarySHA256,
		"requiredRuntimeStorage":         fmt.Sprintf("7 x %s encrypted gp3, delayed binding", input.Runtime.Capacity),
	}, nil
}

func portableQualificationWorkloads(workloads map[string]any, controllerImage string) (map[string]any, error) {
	controller := SecureLocalProfile().ControllerImage
	// Work on a JSON-normalized copy; never mutate the original runner/profile.
	raw, _ := json.Marshal(workloads)
	var portable map[string]any
	if json.Unmarshal(raw, &portable) != nil {
		return nil, errors.New("invalid qualification workload manifest")
	}
	var adapt func(any)
	adapt = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if x["image"] == controller {
				x["image"] = controllerImage
			}
			if x["name"] == "SMP_CONFIG_INIT_IMAGE" {
				x["value"] = controllerImage
			}
			if x["imagePullPolicy"] == "Never" {
				x["imagePullPolicy"] = "IfNotPresent"
			}
			for _, child := range x {
				adapt(child)
			}
		case []any:
			for _, child := range x {
				adapt(child)
			}
		}
	}
	adapt(portable)
	return portable, nil
}
