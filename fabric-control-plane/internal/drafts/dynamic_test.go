package drafts

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sathonate.local/fabric-control-plane/internal/domain"
)

func TestDynamicConfigurationSurvivesRevisionPersistenceAndCopy(t *testing.T) {
	raw, e := os.ReadFile("../../examples/dynamic/b.json")
	if e != nil {
		t.Fatal(e)
	}
	s, e := domain.Decode(strings.NewReader(string(raw)))
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "drafts.json")
	store, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	d, e := store.Save(s, "", 0)
	if e != nil {
		t.Fatal(e)
	}
	d.Desired.Spec.Configuration.Authorities[0].Roles[0] = "mutated"
	stored, e := store.Get(d.ID)
	if e != nil || stored.Desired.Spec.Configuration.Authorities[0].Roles[0] != "signing" {
		t.Fatal("nested configuration leaked mutable state")
	}
	stored.Desired.Spec.Configuration.Chaincodes[0].Version = "2.0"
	updated, e := store.Save(stored.Desired, d.ID, 1)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = store.Save(s, d.ID, 1); !errors.Is(e, ErrConflict) {
		t.Fatal("stale configuration overwritten")
	}
	reopened, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	persisted, e := reopened.Get(d.ID)
	if e != nil {
		t.Fatal(e)
	}
	want, _ := json.Marshal(updated.Desired.Spec.Configuration)
	got, _ := json.Marshal(persisted.Desired.Spec.Configuration)
	if string(want) != string(got) || persisted.Revision != 2 {
		t.Fatal("customer configuration lost during persistence")
	}
}

func TestDynamicConfigurationDoesNotAcceptPrivateMaterialOrTemplates(t *testing.T) {
	raw, e := os.ReadFile("../../examples/dynamic/a.json")
	if e != nil {
		t.Fatal(e)
	}
	for _, key := range []string{"privateKey", "enrollmentSecret", "template", "command"} {
		t.Run(key, func(t *testing.T) {
			var payload map[string]any
			if json.Unmarshal(raw, &payload) != nil {
				t.Fatal("fixture")
			}
			configuration := payload["spec"].(map[string]any)["configuration"].(map[string]any)
			configuration[key] = "forbidden-input"
			data, _ := json.Marshal(payload)
			if _, e = domain.Decode(strings.NewReader(string(data))); e == nil {
				t.Fatal("private/executable field accepted")
			}
		})
	}
}
