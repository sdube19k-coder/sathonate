package deployment

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sathonate.local/fabric-control-plane/internal/domain"
)

// SecureProfile describes verified controller inputs, NOT a qualified application
// worker. No executable flag may be derived from the controller's lab result.
type SecureProfile struct {
	ID                     string   `json:"id"`
	ControllerRevision     string   `json:"controllerRevision"`
	BevelCommit            string   `json:"bevelCommit"`
	ControllerImage        string   `json:"controllerImage"`
	ControllerBinarySHA256 string   `json:"controllerBinarySHA256"`
	BuildManifestSHA256    string   `json:"buildManifestSHA256"`
	StateDatabase          string   `json:"stateDatabase"`
	ExecutionEnabled       bool     `json:"executionEnabled"`
	ProductionEnabled      bool     `json:"productionEnabled"`
	AWSEnabled             bool     `json:"awsEnabled"`
	Blockers               []string `json:"blockers"`
}

func SecureLocalProfile() SecureProfile {
	return SecureProfile{
		ID:                     domain.SecureLocalProfile,
		ControllerRevision:     "1c7fca651f833020184b089544ad1b0359aafb7a",
		BevelCommit:            "cd236ccc61d1d159b3dadb4625d08bb5682a8451",
		ControllerImage:        "sathonate/controller-candidate@sha256:427b352b20576258efdb59c9fc266ec01fe7b7f1419ce1e5d7ead488b10cba6c",
		ControllerBinarySHA256: "e1347172346b64af8a9e96408cc3255f8db089c2b866edc3cbc56cd9c15deaf1",
		BuildManifestSHA256:    "432d9f9ce0811e5c7b028612bb07964cfb547f8a54884f2690295f1b3830939b",
		StateDatabase:          "leveldb",
		Blockers:               []string{"explicit local bearer authentication, PostgreSQL and trusted secure runner configuration are required", "API-to-Fabric qualification must match the selected worker build"},
	}
}

// SecureNamespace accepts only server-resolved identifiers, not a namespace from
// request JSON. A JSON tuple prevents tenant/project delimiter aliases.
func SecureNamespace(tenant, project, network string) (string, error) {
	if tenant == "" || project == "" || network == "" || len(tenant) > 128 || len(project) > 128 || len(network) > 128 {
		return "", errors.New("authenticated tenant, project and network binding required")
	}
	raw, _ := json.Marshal([3]string{tenant, project, network})
	sum := sha256.Sum256(raw)
	return "smp-s-" + hex.EncodeToString(sum[:16]), nil
}

// ValidateSecureNamespace is read-only. It cannot create/adopt a namespace or
// replace its labels; an existing namespace must match every ownership binding.
func ValidateSecureNamespace(labels map[string]string, tenant, project, network string) error {
	if _, err := SecureNamespace(tenant, project, network); err != nil {
		return err
	}
	for key, value := range map[string]string{"fabric.sathonate.io/tenant": tenant, "fabric.sathonate.io/project": project, "fabric.sathonate.io/network": network, "fabric.sathonate.io/profile": domain.SecureLocalProfile} {
		if labels[key] != value {
			return errors.New("exclusive namespace ownership does not match authenticated deployment")
		}
	}
	return nil
}

var ErrSecureIntegrationBlocked = errors.New("secure-local-v1 requires the explicit authenticated secure runner; original-controller fallback is forbidden")

// Reject before operation creation or any Kubernetes call. This is deliberately
// not an implementation of Runner: no disabled scaffold can report success.
func rejectUnintegratedSecureProfile(s domain.NetworkSpec) error {
	if s.Spec.Deployment.TargetRef == domain.SecureLocalProfile || s.Spec.Deployment.TargetRef == domain.SecureEKSProfile {
		return ErrSecureIntegrationBlocked
	}
	return nil
}
