package rollout

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/identity"
	"sathonate.local/fabric-control-plane/internal/maintenance"
)

type fixtureDriver struct {
	locked      bool
	observation Snapshot
	writes      int
	patch       string
}

func (d *fixtureDriver) Lock(context.Context, maintenance.Binding) (func(), error) {
	if d.locked {
		return nil, errors.New("fixture already locked")
	}
	d.locked = true
	return func() { d.locked = false }, nil
}
func (d *fixtureDriver) Observe(context.Context, maintenance.Binding) (Snapshot, error) {
	return d.observation, nil
}
func (d *fixtureDriver) ChangeImage(_ context.Context, plan Plan) error {
	if !d.locked {
		return errors.New("fixture mutation requires a lock")
	}
	d.writes++
	patch, err := ImagePatch(plan)
	d.patch = string(patch)
	return err
}

func TestUpgradeApprovalAndQuorum(t *testing.T) {
	now := time.Now()
	oldImage := "example.test/fabric@sha256:" + strings.Repeat("a", 64)
	newImage := "example.test/fabric@sha256:" + strings.Repeat("b", 64)
	storage := strings.Repeat("c", 64)
	spec := domain.NetworkSpec{Spec: domain.Topology{
		Configuration: &domain.Configuration{Authorities: []domain.Authority{{ID: "ca"}}},
		Ordering:      domain.Ordering{Consensus: "raft"},
		Nodes:         []domain.Node{{ID: "first", Role: "orderer"}, {ID: "second", Role: "orderer"}, {ID: "third", Role: "orderer"}, {ID: "peer", Role: "peer"}},
		Channels:      []domain.Channel{{ID: "customer", OrdererRefs: []string{"first", "second", "third"}, PeerRefs: []string{"peer"}}},
	}}
	admin := identity.Principal{Subject: "verified", TenantID: "tenant", ProjectID: "project", Roles: []string{"admin"}, Scopes: []string{"fabric.write"}}
	binding := maintenance.Binding{TenantID: admin.StorageTenant(), NetworkID: "net", TargetSHA256: storage, NetworkSHA256: topologyDigest(spec)}
	d := &fixtureDriver{observation: Snapshot{ObservedAt: now, BindingSHA256: binding.Digest(), NamespaceUID: "ns-uid", LedgerVerified: true, BackupVerified: true, Channels: []ChannelState{{ID: "customer", ActiveVoters: []string{"first", "second", "third"}, LedgerSHA256: storage}}}}
	for _, entry := range append(spec.Spec.Nodes, domain.Node{ID: "ca", Role: "ca"}) {
		d.observation.Components = append(d.observation.Components, Component{ID: entry.ID, Kind: entry.Role, Namespace: "test", Deployment: entry.ID, UID: entry.ID + "-uid", ResourceVersion: "10", Container: "fabric", Image: oldImage, StorageIdentitySHA256: storage, Ready: true})
	}
	profile := Profile{Edges: map[string]map[string][]string{
		"orderer": {oldImage: {newImage}}, "peer": {oldImage: {newImage}}, "ca": {oldImage: {newImage}},
	}}
	for _, target := range []string{"first", "peer", "ca"} {
		d.observation.EnrollmentPaused = target == "ca"
		plan, err := Preview(admin, binding, spec, d.observation, target, newImage, profile, now)
		if err != nil {
			t.Fatal(err)
		}
		d.observation.Components[0].ResourceVersion = "11"
		if target == "first" && Submit(context.Background(), admin, binding, spec, profile, target, newImage, plan.Digest, now.Add(time.Minute), now, d) == nil {
			t.Fatal("stale resource version accepted")
		}
		d.observation.Components[0].ResourceVersion = "10"
		if err := Submit(context.Background(), admin, binding, spec, profile, target, newImage, plan.Digest, now.Add(time.Minute), now, d); err != nil {
			t.Fatal(err)
		}
		for _, part := range []string{"/metadata/uid", "/metadata/resourceVersion", "/0/name", "/0/image", newImage} {
			if !strings.Contains(d.patch, part) {
				t.Fatalf("patch lacks %s", part)
			}
		}
	}
	plan, err := Preview(admin, binding, spec, d.observation, "first", newImage, profile, now)
	if err != nil {
		t.Fatal(err)
	}
	d.locked = true
	if Submit(context.Background(), admin, binding, spec, profile, "first", newImage, plan.Digest, now.Add(time.Minute), now, d) == nil {
		t.Fatal("concurrent mutation was accepted")
	}
	d.locked = false
	if Submit(context.Background(), admin, binding, spec, profile, "first", newImage, plan.Digest, now, now, d) == nil {
		t.Fatal("expired approval was accepted")
	}
	if d.writes != 3 {
		t.Fatalf("writes = %d", d.writes)
	}
	for _, mutate := range []func(*Snapshot){
		func(s *Snapshot) { s.BindingSHA256 = storage },
		func(s *Snapshot) { s.ObservedAt = now.Add(-2 * time.Minute) },
		func(s *Snapshot) {
			s.Channels = []ChannelState{{ID: "customer", ActiveVoters: []string{"first", "second", "foreign"}, LedgerSHA256: storage}}
		},
		func(s *Snapshot) { s.BackupVerified = false },
		func(s *Snapshot) { s.Components[1].Ready = false },
		func(s *Snapshot) { s.Components[1].Rolling = true },
		func(s *Snapshot) { s.Components[1].UID = s.Components[0].UID },
	} {
		invalid := d.observation
		invalid.Components = append([]Component(nil), invalid.Components...)
		mutate(&invalid)
		if _, err := Preview(admin, binding, spec, invalid, "first", newImage, profile, now); err == nil {
			t.Fatal("unsafe observation accepted")
		}
	}
	d.observation.EnrollmentPaused = false
	if _, err := Preview(admin, binding, spec, d.observation, "ca", newImage, profile, now); err == nil {
		t.Fatal("CA enrolled during upgrade")
	}
	foreign := admin
	foreign.ProjectID = "other"
	if _, err := Preview(foreign, binding, spec, d.observation, "first", newImage, profile, now); err == nil {
		t.Fatal("cross-project preview accepted")
	}
	spec.Spec.Channels[0].OrdererRefs = []string{"first", "second"}
	d.observation.Channels[0].ActiveVoters = []string{"first", "second"}
	binding.NetworkSHA256 = topologyDigest(spec)
	d.observation.BindingSHA256 = binding.Digest()
	if _, err := Preview(admin, binding, spec, d.observation, "first", newImage, profile, now); err == nil {
		t.Fatal("quorum loss accepted")
	}
}
