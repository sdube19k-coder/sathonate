package deployment

import (
	"context"
	"errors"
	"os"
	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/kubernetesfabric"
	"testing"
)

func secureDesign(t *testing.T) domain.NetworkSpec {
	t.Helper()
	f, e := os.Open("../../examples/secure-local-network.json")
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	s, e := domain.Decode(f)
	if e != nil {
		t.Fatal(e)
	}
	return s
}

func TestSecureProfileCannotFallBackOrCreateOperation(t *testing.T) {
	m, err := Open(t.TempDir(), MockRunner{})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if _, err = m.Start("tenant", "actor", "record", secureDesign(t), 1, "secure-request-key", "", ""); !errors.Is(err, ErrSecureIntegrationBlocked) {
		t.Fatal("secure request reached legacy dispatch", err)
	}
	if len(m.Operations().List("tenant")) != 0 {
		t.Fatal("blocked secure request created an operation")
	}
	emitted := false
	if _, err = (KubernetesRunner{}).Run(context.Background(), Request{Network: secureDesign(t)}, func(string, string) error { emitted = true; return nil }); !errors.Is(err, ErrSecureIntegrationBlocked) || emitted {
		t.Fatal("direct legacy runner bypassed secure boundary", err)
	}
	if err = kubernetesfabric.ValidateProfile(secureDesign(t)); err == nil {
		t.Fatal("legacy worker accepted secure input")
	}
}

func TestSecureNamespaceRequiresEveryOwnershipBinding(t *testing.T) {
	one, e := SecureNamespace("tenant/a", "project", "network")
	if e != nil {
		t.Fatal(e)
	}
	two, _ := SecureNamespace("tenant", "a/project", "network")
	if one == two {
		t.Fatal("delimiter alias crossed namespace binding")
	}
	for _, binding := range [][3]string{{"tenant/b", "project", "network"}, {"tenant/a", "other-project", "network"}, {"tenant/a", "project", "other-network"}} {
		ns, _ := SecureNamespace(binding[0], binding[1], binding[2])
		if ns == one {
			t.Fatal("ownership binding alias")
		}
	}
	if _, e = SecureNamespace("tenant", "", "network"); e == nil {
		t.Fatal("missing project accepted")
	}
	labels := map[string]string{"fabric.sathonate.io/tenant": "tenant", "fabric.sathonate.io/project": "project", "fabric.sathonate.io/network": "network", "fabric.sathonate.io/profile": domain.SecureLocalProfile}
	if e = ValidateSecureNamespace(labels, "tenant", "project", "network"); e != nil {
		t.Fatal(e)
	}
	for key := range labels {
		saved := labels[key]
		labels[key] = "foreign"
		if ValidateSecureNamespace(labels, "tenant", "project", "network") == nil {
			t.Fatal("foreign namespace accepted", key)
		}
		labels[key] = saved
	}
}

func TestSecureControllerQualificationDoesNotEnableApplication(t *testing.T) {
	p := SecureLocalProfile()
	if p.ExecutionEnabled || p.ProductionEnabled || p.AWSEnabled || len(p.Blockers) == 0 || p.StateDatabase != "leveldb" {
		t.Fatal("controller qualification incorrectly enabled application execution")
	}
	// Return fresh blocker slices; callers cannot mutate the advertised gate.
	p.Blockers[0] = "changed"
	if SecureLocalProfile().Blockers[0] == "changed" {
		t.Fatal("profile is mutable across requests")
	}
}
