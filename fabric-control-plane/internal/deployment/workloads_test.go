package deployment

import (
	"encoding/json"
	"strings"
	"testing"
)

func workloadPayloads(t *testing.T, pod map[string]any, claims ...map[string]any) ([]byte, []byte) {
	t.Helper()
	p, _ := json.Marshal(map[string]any{"apiVersion": "v1", "kind": "PodList", "items": []any{pod}})
	v, _ := json.Marshal(map[string]any{"apiVersion": "v1", "kind": "PersistentVolumeClaimList", "items": claims})
	return p, v
}

func TestWorkloadProjectionKeepsOnlyCurrentOperationalFields(t *testing.T) {
	pod := map[string]any{"metadata": map[string]any{"name": "peer-pod", "namespace": "owned", "labels": map[string]string{"release": "peer-a"}, "annotations": map[string]string{"private": "private-annotation-canary"}}, "spec": map[string]any{"containers": []any{map[string]any{"env": []any{map[string]string{"name": "PASSWORD", "value": "private-env-canary"}}, "args": []string{"private-argv-canary"}}}, "volumes": []any{map[string]any{"persistentVolumeClaim": map[string]string{"claimName": "peer-data"}}, map[string]any{"secret": map[string]string{"secretName": "private-reference-canary"}}}}, "status": map[string]any{"phase": "Running", "conditions": []any{map[string]string{"type": "Ready", "status": "True"}}, "message": "private-status-canary", "containerStatuses": []any{map[string]any{"ready": true, "restartCount": 2, "lastState": map[string]any{"terminated": map[string]string{"reason": "OOMKilled", "message": "private-error-canary"}}}}}}
	claim := map[string]any{"metadata": map[string]string{"name": "peer-data", "namespace": "owned"}, "status": map[string]string{"phase": "Bound", "message": "private-volume-canary"}}
	p, v := workloadPayloads(t, pod, claim)
	out, e := projectWorkloads("owned", map[string]string{"peer-a": "peer"}, p, v)
	if e != nil || !out.ReadOnly || out.LedgerChecked || !out.WorkloadsReady || len(out.Pods) != 1 || len(out.Volumes) != 1 || out.Pods[0].Restarts != 2 || out.Pods[0].LastExitReason != "OOMKilled" {
		t.Fatal("current observation lost or fabricated readiness", e)
	}
	raw, _ := json.Marshal(out)
	for _, forbidden := range []string{"private-", "secretName", "env", "args", "annotations", `"message":`} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatal("operational projection exposed excluded fields", forbidden)
		}
	}
}

func TestWorkloadProjectionDistinguishesFailureMissingAndStorage(t *testing.T) {
	pod := map[string]any{"metadata": map[string]any{"name": "peer-pod", "namespace": "owned", "labels": map[string]string{"release": "peer-a"}}, "spec": map[string]any{"volumes": []any{map[string]any{"persistentVolumeClaim": map[string]string{"claimName": "peer-data"}}}}, "status": map[string]any{"phase": "Pending", "containerStatuses": []any{map[string]any{"ready": false, "restartCount": 4, "state": map[string]any{"waiting": map[string]string{"reason": "CrashLoopBackOff", "message": "private-failure-canary"}}}}}}
	p, v := workloadPayloads(t, pod)
	out, e := projectWorkloads("owned", map[string]string{"peer-a": "peer", "missing-orderer": "orderer"}, p, v)
	if e != nil || out.WorkloadsReady || len(out.Missing) != 2 || out.Pods[0].WaitingReason != "CrashLoopBackOff" || out.Volumes[0].Phase != "Missing" || len(out.Guidance) < 2 {
		t.Fatal("failure/recovery visibility missing", e)
	}
	if workloadReason("private-arbitrary-reason") != "Other" || workloadPhase("private-arbitrary-phase") != "Unknown" {
		t.Fatal("unknown provider text escaped allowlist")
	}
}

func TestWorkloadProjectionRejectsForeignScopeAndWrongPayload(t *testing.T) {
	pod := map[string]any{"metadata": map[string]any{"name": "foreign", "namespace": "foreign", "labels": map[string]string{"release": "peer-a"}}, "status": map[string]string{"phase": "Running"}}
	p, v := workloadPayloads(t, pod)
	if _, e := projectWorkloads("owned", map[string]string{"peer-a": "peer"}, p, v); e == nil {
		t.Fatal("foreign namespace accepted")
	}
	if _, e := projectWorkloads("owned", map[string]string{"peer-a": "peer"}, []byte(`{"apiVersion":"v1","kind":"Status"}`), v); e == nil {
		t.Fatal("error payload treated as healthy inventory")
	}
	pod["metadata"] = map[string]any{"name": "foreign-resource", "namespace": "owned", "labels": map[string]string{"release": "foreign-resource"}}
	p, v = workloadPayloads(t, pod)
	out, e := projectWorkloads("owned", map[string]string{"peer-a": "peer"}, p, v)
	if e != nil || len(out.Pods) != 0 || out.WorkloadsReady || len(out.Missing) != 1 {
		t.Fatal("foreign resource included or missing resources reported ready", e)
	}
}

func TestWorkloadProjectionAcceptsKubectlListsButRejectsWrongItemTypes(t *testing.T) {
	pod := map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": "peer-pod", "namespace": "owned", "labels": map[string]string{"release": "peer-a"}}, "status": map[string]any{"phase": "Running", "conditions": []any{map[string]string{"type": "Ready", "status": "True"}}, "containerStatuses": []any{map[string]any{"ready": true}}}}
	p, v := workloadPayloads(t, pod)
	p = []byte(strings.Replace(string(p), `"kind":"PodList"`, `"kind":"List"`, 1))
	v = []byte(strings.Replace(string(v), `"kind":"PersistentVolumeClaimList"`, `"kind":"List"`, 1))
	if out, e := projectWorkloads("owned", map[string]string{"peer-a": "peer"}, p, v); e != nil || !out.WorkloadsReady {
		t.Fatal("actual kubectl generic List framing was rejected", e)
	}
	p = []byte(strings.Replace(string(p), `"kind":"Pod"`, `"kind":"Secret"`, 1))
	if _, e := projectWorkloads("owned", map[string]string{"peer-a": "peer"}, p, v); e == nil {
		t.Fatal("wrong object type was accepted as a workload")
	}
}

func TestWorkloadProjectionHonorsPodReadinessGate(t *testing.T) {
	pod := map[string]any{"metadata": map[string]any{"name": "peer", "namespace": "owned", "labels": map[string]string{"release": "peer-a"}}, "status": map[string]any{"phase": "Running", "conditions": []any{map[string]string{"type": "Ready", "status": "False"}}, "containerStatuses": []any{map[string]any{"ready": true}}}}
	p, v := workloadPayloads(t, pod)
	out, e := projectWorkloads("owned", map[string]string{"peer-a": "peer"}, p, v)
	if e != nil || out.WorkloadsReady || out.Pods[0].Ready {
		t.Fatal("container readiness bypassed the pod readiness gate", e)
	}
}
