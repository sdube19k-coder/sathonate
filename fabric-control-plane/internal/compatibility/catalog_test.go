package compatibility

import (
	"encoding/json"
	"testing"
)

func TestCatalogDeterministicAndIsolated(t *testing.T) {
	catalog := Snapshot()
	before, _ := json.Marshal(catalog)
	protocol := catalog.Protocols[0]
	if protocol.ID != "drunix" || protocol.Version != "1.0.0" ||
		protocol.SourceCommit != "3fb2b135a5b2adf7e1071d9b1624052a1bbb1772" {
		t.Fatal("Drunix release was not pinned")
	}
	capabilities := make(map[string]bool)
	for _, capability := range protocol.Capabilities {
		if capability.Evidence == "" || capabilities[capability.ID] {
			t.Fatal("capability evidence missing or entry duplicated")
		}
		capabilities[capability.ID] = true
	}
	for _, id := range []string{"split-peer-roles", "stateless-validation", "yugabytedb-state", "keydb-transient-store"} {
		if !capabilities[id] {
			t.Fatalf("missing %s evidence", id)
		}
	}
	catalog.Protocols[0].Capabilities[0].ID = "modified"
	catalog.Combinations[0].Status = Qualified
	after, _ := json.Marshal(Snapshot())
	if string(before) != string(after) {
		t.Fatal("catalog changed between reads")
	}
}

func TestCompatibilityRequiresExactCombination(t *testing.T) {
	seen := make(map[Combination]bool)
	for _, entry := range Snapshot().Combinations {
		if seen[entry.Combination] || Decide(entry.Combination) != entry {
			t.Fatal("duplicate or unresolved catalog entry")
		}
		seen[entry.Combination] = true
		if entry.Protocol == "drunix" && entry.Status != ReferenceOnly {
			t.Fatal("Drunix must remain reference-only")
		}
		for _, edit := range []func(*Combination){
			func(k *Combination) { k.Protocol = "unknown" },
			func(k *Combination) { k.ProtocolVersion = "latest" },
			func(k *Combination) { k.Engine = "falcon" },
			func(k *Combination) { k.EngineVersion = "latest" },
			func(k *Combination) { k.Target = "unregistered-kubernetes" },
		} {
			key := entry.Combination
			edit(&key)
			if Decide(key).Status != Unsupported {
				t.Fatalf("unexpected support for %+v", key)
			}
		}
	}
	for _, engine := range []string{"bevel", "falcon"} {
		if Decide(Combination{"drunix", "1.0.0", engine, "1.0.0", "kubernetes"}).Status != Unsupported {
			t.Fatalf("Drunix must not assume %s compatibility", engine)
		}
	}
}
