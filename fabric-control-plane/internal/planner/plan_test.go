package planner

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"sathonate.local/fabric-control-plane/internal/domain"
)

func load(t *testing.T, name string) domain.NetworkSpec {
	t.Helper()
	f, e := os.Open("../../examples/" + name)
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

func TestPlanIdentityAndDependencies(t *testing.T) {
	s := load(t, "native-network.json")
	a, e := Create(s)
	if e != nil {
		t.Fatal(e)
	}
	b, e := Create(s)
	if e != nil {
		t.Fatal(e)
	}
	if a.ID != b.ID || a.ID == "" {
		t.Fatal("plan digest is not deterministic")
	}
	if a.Executable {
		t.Fatal("planning scaffold must not claim execution readiness")
	}
	seen := map[string]bool{}
	for _, step := range a.Steps {
		for _, dep := range step.DependsOn {
			if !seen[dep] {
				t.Fatal("dependency not previously declared")
			}
		}
		if seen[step.ID] {
			t.Fatal("duplicate step")
		}
		seen[step.ID] = true
	}
	s.Spec.Nodes[0].PlacementRef = "host-d"
	c, e := Create(s)
	if e != nil {
		t.Fatal(e)
	}
	if a.ID == c.ID {
		t.Fatal("changed placement must change plan identity")
	}
}

func TestEngineBoundaries(t *testing.T) {
	for _, name := range []string{"native-network.json", "cloud-network.json"} {
		s := load(t, name)
		p, e := Create(s)
		if e != nil {
			t.Fatal(e)
		}
		b, _ := json.Marshal(p.Steps)
		if p.Engine == "native" && strings.Contains(strings.ToLower(string(b)), "bevel") {
			t.Fatal("native plan depends on Bevel")
		}
		if p.Engine == "bevel" && !strings.Contains(string(b), "bevel-network") {
			t.Fatal("cloud plan bypasses Bevel")
		}
	}
}

func TestInvalidSpecNeverProducesPlan(t *testing.T) {
	s := load(t, "native-network.json")
	s.Spec.Channels[0].OrdererRefs = nil
	if _, e := Create(s); e == nil {
		t.Fatal("invalid spec produced a plan")
	}
}
