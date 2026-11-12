package customer

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/topology"
)

func TestProvisionPreviewBindsTargetAndDraft(t *testing.T) {
	file, err := os.Open("../../examples/dynamic/a.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	spec, err := domain.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	spec.Spec.Deployment.Provider = domain.CustomerKubernetesProvider
	spec.Spec.Deployment.TargetRef = "customer-target"
	actor := Actor{"https://id.test", "owner", "tenant", "project"}
	store := &fixtureStore{map[string]Account{"tenant": {Owner: actor.identity(), Projects: []Project{{ID: "project", State: "active"}},
		Subscription: Subscription{Plan: "team", Status: "active", Until: time.Now().Add(time.Hour)}}}, map[string]Change{}}
	svc := Service{Store: store}
	target := Target{Ref: spec.Spec.Deployment.TargetRef, Tenant: actor.Tenant, Project: actor.Project, ClusterSHA256: strings.Repeat("a", 64),
		Namespace: "customer-test", NamespaceUID: "fixture-uid", Storage: map[string]topology.StorageCapability{}}
	for _, n := range spec.Spec.Configuration.Nodes {
		target.Storage[n.Storage.Class] = topology.StorageCapability{Provisioner: "fixture.test/csi", Encrypted: true, WaitForFirstConsumer: true, MaxCapacityGiB: 100}
	}
	for _, ca := range spec.Spec.Configuration.Authorities {
		target.Storage[ca.Storage.Class] = topology.StorageCapability{Provisioner: "fixture.test/csi", Encrypted: true, WaitForFirstConsumer: true, MaxCapacityGiB: 100}
	}
	ctx := context.Background()
	first, err := svc.PreviewProvision(ctx, actor, Targets{target}, "network-record", 1, spec)
	if err != nil || first.ExecutionEnabled || len(first.Storage) != len(spec.Spec.Nodes)+len(spec.Spec.Configuration.Authorities) {
		t.Fatalf("preview %+v %v", first, err)
	}
	next, err := svc.PreviewProvision(ctx, actor, Targets{target}, "network-record", 2, spec)
	if err != nil || next.BindingSHA256 == first.BindingSHA256 {
		t.Fatal("revision missing from binding")
	}
	target.NamespaceUID = "changed"
	next, err = svc.PreviewProvision(ctx, actor, Targets{target}, "network-record", 1, spec)
	if err != nil || next.BindingSHA256 == first.BindingSHA256 {
		t.Fatal("target UID missing from binding")
	}
	target.Project = "foreign"
	if _, err := svc.PreviewProvision(ctx, actor, Targets{target}, "network-record", 1, spec); err != ErrDenied {
		t.Fatal("foreign target accepted")
	}
	raw, _ := json.Marshal(Targets{target, target})
	if _, err := LoadTargets(bytes.NewReader(raw)); err != ErrInvalid {
		t.Fatal("duplicate target accepted")
	}
	raw, _ = json.Marshal(Targets{target})
	if _, err := LoadTargets(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
}
