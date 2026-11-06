package deployment

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"sathonate.local/fabric-control-plane/internal/kubernetesfabric"
)

var ErrWorkloadsUnavailable = errors.New("current workload observations are unavailable for this approved local target")

type WorkloadObservation struct {
	Name           string `json:"name"`
	Component      string `json:"component"`
	Kind           string `json:"kind"`
	Phase          string `json:"phase"`
	Ready          bool   `json:"ready"`
	Restarts       int64  `json:"restarts"`
	WaitingReason  string `json:"waitingReason,omitempty"`
	LastExitReason string `json:"lastExitReason,omitempty"`
}
type VolumeObservation struct {
	Name  string `json:"name"`
	Phase string `json:"phase"`
}
type WorkloadSnapshot struct {
	OperationID    string                `json:"operationId"`
	Revision       int                   `json:"revision"`
	CheckedAt      time.Time             `json:"checkedAt"`
	ReadOnly       bool                  `json:"readOnly"`
	LedgerChecked  bool                  `json:"ledgerChecked"`
	WorkloadsReady bool                  `json:"workloadsReady"`
	Pods           []WorkloadObservation `json:"pods"`
	Volumes        []VolumeObservation   `json:"volumes"`
	Missing        []string              `json:"missing"`
	Guidance       []string              `json:"guidance"`
}

func (m *Manager) Workloads(ctx context.Context, tenant, id string) (WorkloadSnapshot, error) {
	op, e := m.jobs.Get(tenant, id)
	if e != nil {
		return WorkloadSnapshot{}, e
	}
	if m.history != nil || m.Mode() != "secure-local" {
		return WorkloadSnapshot{}, ErrWorkloadsUnavailable
	}
	s, e := m.Status(tenant, id)
	if e != nil || s.Simulated || s.Mode != "secure-local" || s.Phase == "superseded" || s.Phase == "removed" {
		return WorkloadSnapshot{}, ErrWorkloadsUnavailable
	}
	reader, ok := m.runner.(interface {
		InspectWorkloads(context.Context, Request) (WorkloadSnapshot, error)
	})
	if !ok {
		return WorkloadSnapshot{}, ErrWorkloadsUnavailable
	}
	out, e := reader.InspectWorkloads(ctx, Request{TenantID: tenant, NetworkID: op.NetworkID, OperationID: id, Network: s.Network, Lifecycle: s.Lifecycle, TargetSHA256: s.TargetSHA256})
	if e != nil || !out.ReadOnly || out.LedgerChecked || out.CheckedAt.IsZero() {
		return WorkloadSnapshot{}, ErrWorkloadsUnavailable
	}
	out.OperationID, out.Revision = id, s.Revision
	return out, nil
}

// This adapter issues only scoped GETs. Existing target and namespace ownership
// checks apply even to failed operations; no Job, Pod, PVC or material is changed.
func (r SecureRunner) InspectWorkloads(ctx context.Context, q Request) (WorkloadSnapshot, error) {
	if r.binding == nil || r.binding.remote != nil || r.Validate(q) != nil || r.owned(ctx, q.TenantID, q.NetworkID) != nil {
		return WorkloadSnapshot{}, ErrWorkloadsUnavailable
	}
	b := r.base()
	ns := b.namespace(q.TenantID, q.NetworkID)
	pods, e := b.run(ctx, "-n", ns, "get", "pods", "-o", "json")
	if e != nil {
		return WorkloadSnapshot{}, ErrWorkloadsUnavailable
	}
	volumes, e := b.run(ctx, "-n", ns, "get", "persistentvolumeclaims", "-o", "json")
	if e != nil {
		return WorkloadSnapshot{}, ErrWorkloadsUnavailable
	}
	s := q.Network
	if q.Lifecycle != nil {
		s = q.Lifecycle.Effective(s)
	}
	expected := map[string]string{"secure-controller": "controller"}
	for _, n := range s.Spec.Nodes {
		expected[n.ID] = n.Role
	}
	if s.Spec.Configuration != nil {
		for _, a := range s.Spec.Configuration.Authorities {
			expected[a.ID] = "authority"
		}
		for _, cc := range s.Spec.Configuration.Chaincodes {
			expected[kubernetesfabric.DynamicService(cc.ChannelRef, cc.Name, cc.Sequence)] = "chaincode"
		}
	} else {
		for _, org := range s.Spec.Organizations {
			expected[org.ID+"-ca"] = "authority"
		}
		expected["smp-smoke"] = "chaincode"
	}
	return projectWorkloads(ns, expected, pods, volumes)
}

// Typed projection deliberately excludes arguments, env, Secret references,
// annotations, logs, event/status messages and Kubernetes error text.
func projectWorkloads(namespace string, expected map[string]string, pods, volumes []byte) (WorkloadSnapshot, error) {
	out := WorkloadSnapshot{CheckedAt: time.Now().UTC(), ReadOnly: true, WorkloadsReady: true, Pods: []WorkloadObservation{}, Volumes: []VolumeObservation{}, Missing: []string{}, Guidance: []string{}}
	if len(expected) == 0 || len(pods) > 8<<20 || len(volumes) > 8<<20 {
		return out, ErrWorkloadsUnavailable
	}
	type meta struct {
		Name      string
		Namespace string
		Labels    map[string]string
	}
	type terminated struct{ Reason string }
	type container struct {
		Ready        bool
		RestartCount int64
		State        struct {
			Waiting    *terminated
			Terminated *terminated
		}
		LastState struct{ Terminated *terminated }
	}
	var list struct {
		APIVersion, Kind string
		Items            []struct {
			APIVersion, Kind string
			Metadata         meta
			Spec             struct {
				Volumes []struct{ PersistentVolumeClaim *struct{ ClaimName string } }
			}
			Status struct {
				Phase                 string
				Conditions            []struct{ Type, Status string }
				ContainerStatuses     []container
				InitContainerStatuses []container
			}
		}
	}
	if json.Unmarshal(pods, &list) != nil || list.APIVersion != "v1" || (list.Kind != "PodList" && list.Kind != "List") {
		return out, ErrWorkloadsUnavailable
	}
	seen, ready, claims := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, p := range list.Items {
		if p.Metadata.Namespace != namespace || (p.Kind != "" && p.Kind != "Pod") || (p.APIVersion != "" && p.APIVersion != "v1") {
			return out, ErrWorkloadsUnavailable
		}
		component := p.Metadata.Labels["release"]
		if component == "" {
			component = p.Metadata.Labels["chaincode"]
		}
		if component == "" && p.Metadata.Labels["app"] == "secure-controller" {
			component = "secure-controller"
		}
		kind, ok := expected[component]
		if !ok {
			continue
		}
		if seen[p.Metadata.Name] {
			return out, ErrWorkloadsUnavailable
		}
		seen[p.Metadata.Name] = true
		row := WorkloadObservation{Name: p.Metadata.Name, Component: component, Kind: kind, Phase: workloadPhase(p.Status.Phase), Ready: p.Status.Phase == "Running" && len(p.Status.ContainerStatuses) > 0}
		podReady := false
		for _, condition := range p.Status.Conditions {
			if condition.Type == "Ready" {
				podReady = condition.Status == "True"
			}
		}
		row.Ready = row.Ready && podReady
		for _, c := range p.Status.ContainerStatuses {
			row.Ready = row.Ready && c.Ready
			if c.State.Terminated != nil {
				row.LastExitReason = workloadReason(c.State.Terminated.Reason)
			}
		}
		for _, c := range append(p.Status.ContainerStatuses, p.Status.InitContainerStatuses...) {
			if c.RestartCount < 0 || c.RestartCount > 1<<31-1 {
				return out, ErrWorkloadsUnavailable
			}
			row.Restarts += c.RestartCount
			if c.State.Waiting != nil {
				row.WaitingReason = workloadReason(c.State.Waiting.Reason)
			}
			if c.LastState.Terminated != nil {
				row.LastExitReason = workloadReason(c.LastState.Terminated.Reason)
			}
		}
		for _, v := range p.Spec.Volumes {
			if v.PersistentVolumeClaim != nil {
				claims[v.PersistentVolumeClaim.ClaimName] = true
			}
		}
		if !row.Ready {
			out.WorkloadsReady = false
		}
		ready[component] = ready[component] || row.Ready
		out.Pods = append(out.Pods, row)
	}
	for component := range expected {
		if !ready[component] {
			out.Missing = append(out.Missing, component)
			out.WorkloadsReady = false
		}
	}
	var pvcs struct {
		APIVersion, Kind string
		Items            []struct {
			APIVersion, Kind string
			Metadata         meta
			Status           struct{ Phase string }
		}
	}
	if json.Unmarshal(volumes, &pvcs) != nil || pvcs.APIVersion != "v1" || (pvcs.Kind != "PersistentVolumeClaimList" && pvcs.Kind != "List") {
		return out, ErrWorkloadsUnavailable
	}
	for _, v := range pvcs.Items {
		if v.Metadata.Namespace != namespace || (v.Kind != "" && v.Kind != "PersistentVolumeClaim") || (v.APIVersion != "" && v.APIVersion != "v1") {
			return out, ErrWorkloadsUnavailable
		}
		if !claims[v.Metadata.Name] {
			continue
		}
		phase := v.Status.Phase
		if phase != "Bound" && phase != "Pending" && phase != "Lost" {
			phase = "Unknown"
		}
		out.Volumes = append(out.Volumes, VolumeObservation{Name: v.Metadata.Name, Phase: phase})
		delete(claims, v.Metadata.Name)
		if phase != "Bound" {
			out.WorkloadsReady = false
		}
	}
	for name := range claims {
		out.Volumes = append(out.Volumes, VolumeObservation{Name: name, Phase: "Missing"})
		out.WorkloadsReady = false
	}
	if !out.WorkloadsReady {
		out.Guidance = append(out.Guidance, "Review missing or unready components and volume binding before retrying the recorded operation. Do not create a replacement network.")
	}
	for _, p := range out.Pods {
		if p.WaitingReason != "" || p.LastExitReason == "OOMKilled" {
			out.Guidance = append(out.Guidance, "Check approved images, resource limits and authenticated operation diagnostics. Raw container messages are excluded.")
			break
		}
	}
	out.Guidance = append(out.Guidance, "Workload readiness does not prove ledger consistency. Run an authorized transaction verification for fresh Fabric evidence.")
	sort.Strings(out.Missing)
	sort.Slice(out.Pods, func(i, j int) bool { return out.Pods[i].Name < out.Pods[j].Name })
	sort.Slice(out.Volumes, func(i, j int) bool { return out.Volumes[i].Name < out.Volumes[j].Name })
	return out, nil
}

func workloadPhase(s string) string {
	switch s {
	case "Running", "Pending", "Succeeded", "Failed":
		return s
	}
	return "Unknown"
}
func workloadReason(s string) string {
	switch s {
	case "CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull", "ContainerCreating", "PodInitializing", "CreateContainerConfigError", "CreateContainerError", "RunContainerError", "InvalidImageName", "OOMKilled", "Error", "Completed", "ContainerCannotRun":
		return s
	}
	return "Other"
}
