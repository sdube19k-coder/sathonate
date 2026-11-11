package deployment

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/maintenance"
)

const coldLabel = "fabric.sathonate.io/cold-maintenance"
const coldJournal = "sathonate-cold-checkpoint"

type ColdOptions struct {
	Binding                             *maintenance.Binding
	Tenant, Operation, Action, Artifact string
	Key                                 []byte
}
type coldDeployment struct {
	Name, UID, TemplateHash string
	Replicas                int
}
type coldVolume struct{ Name, UID string }
type coldPlan struct {
	Schema, Namespace, NamespaceUID, TargetSHA256, NetworkSHA256, Tenant, Network, Operation, Digest string
	Deployments                                                                                      []coldDeployment
	Volumes                                                                                          []coldVolume
}
type coldBundle struct {
	Schema    string
	Plan      coldPlan
	Control   map[string]json.RawMessage
	Resources json.RawMessage
	Archives  map[string][]byte
}
type ColdReceipt struct {
	Schema            string               `json:"schema"`
	Action            string               `json:"action"`
	Namespace         string               `json:"namespace"`
	ArtifactSHA256    string               `json:"artifactSHA256,omitempty"`
	Volumes           int                  `json:"volumes"`
	WorkloadsQuiesced bool                 `json:"workloadsQuiesced"`
	Notice            string               `json:"notice"`
	Binding           *maintenance.Binding `json:"binding,omitempty"`
	Operation         string               `json:"operationId,omitempty"`
}

// ColdBackup is deliberately an offline local-administrator workflow. The API
// must be stopped: it holds the same PostgreSQL executor lock. No plaintext
// payloads are written to disk or returned in receipts. A maintenance fence and
// public immutable checkpoint survive interruption. Resume is explicit.
func (r SecureRunner) ColdBackup(ctx context.Context, pool *pgxpool.Pool, o ColdOptions) (ColdReceipt, error) {
	denied := errors.New("cold maintenance refused; verify offline executor, exact binding, checkpoint, storage and private artifact")
	receipt := ColdReceipt{Schema: "sathonate.local-cold-backup/v1", Action: o.Action}
	if pool == nil || !ColdDatabaseAllowed(pool.Config().ConnConfig.Host, pool.Config().ConnConfig.Database) || r.binding == nil || r.binding.remote != nil || len(o.Key) != 32 || (o.Action != "capture" && o.Action != "restore" && o.Action != "resume" && o.Action != "capture-resume") {
		return receipt, denied
	}
	lease, e := pool.Acquire(ctx)
	if e != nil {
		return receipt, denied
	}
	defer lease.Release()
	discardLease := func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = lease.Hijack().Close(closeCtx)
	}
	var locked bool
	if err := lease.QueryRow(ctx, "SELECT pg_try_advisory_lock(718481029)").Scan(&locked); err != nil {
		discardLease()
		return receipt, denied
	}
	if !locked {
		return receipt, errors.New("stop the local API/executor before cold maintenance")
	}
	defer func() {
		unlock, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var released bool
		if err := lease.QueryRow(unlock, "SELECT pg_advisory_unlock(718481029)").Scan(&released); err != nil || !released {
			// A pooled session must never retain an ambiguous executor lease.
			discardLease()
		}
	}()
	if o.Action == "capture-resume" {
		o.Action = "capture"
		captured, err := r.coldBackupWithLease(ctx, lease, o)
		if err != nil {
			return captured, err
		}
		o.Action = "resume"
		resumed, err := r.coldBackupWithLease(ctx, lease, o)
		if err != nil {
			return captured, err
		}
		resumed.Action = "capture-resume"
		resumed.ArtifactSHA256 = captured.ArtifactSHA256
		return resumed, nil
	}
	return r.coldBackupWithLease(ctx, lease, o)
}

func (r SecureRunner) coldBackupWithLease(ctx context.Context, lease *pgxpool.Conn, o ColdOptions) (ColdReceipt, error) {
	denied := errors.New("cold maintenance refused; verify offline executor, exact binding, checkpoint, storage and private artifact")
	receipt := ColdReceipt{Schema: "sathonate.local-cold-backup/v1", Action: o.Action}
	tx, e := lease.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if e != nil {
		return receipt, denied
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT set_config('sathonate.tenant_id',$1,true)", o.Tenant); e != nil {
		return receipt, denied
	}
	var state Result
	var raw []byte
	var record string
	var active bool
	if o.Action == "restore" && o.Binding != nil {
		if e = r.restoreColdControl(ctx, tx, o); e != nil {
			return receipt, e
		}
	}
	if tx.QueryRow(ctx, "SELECT network_id FROM sathonate_operations WHERE tenant_id=$1 AND id=$2 AND status='succeeded'", o.Tenant, o.Operation).Scan(&record) != nil || tx.QueryRow(ctx, "SELECT payload FROM sathonate_deployments WHERE tenant_id=$1 AND operation_id=$2", o.Tenant, o.Operation).Scan(&raw) != nil || json.Unmarshal(raw, &state) != nil || !state.Verified || state.LocalDeletion != nil {
		return receipt, denied
	}
	if tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM sathonate_operations WHERE tenant_id=$1 AND network_id=$2 AND (status='running' OR (payload->>'createdAt')::timestamptz>(SELECT (payload->>'createdAt')::timestamptz FROM sathonate_operations WHERE tenant_id=$1 AND id=$3)))", o.Tenant, record, o.Operation).Scan(&active) != nil || active {
		return receipt, denied
	}
	q := Request{TenantID: o.Tenant, NetworkID: record, OperationID: o.Operation, Network: state.Network, TargetSHA256: state.TargetSHA256}
	if r.Validate(q) != nil || q.Network.Spec.Deployment.TargetRef != domain.DynamicLocalProfile || q.Network.Spec.Configuration == nil {
		return receipt, denied
	}
	b := r.base()
	ns := b.namespace(q.TenantID, q.NetworkID)
	receipt.Namespace = ns
	if b.validateSecure(ctx) != nil {
		return receipt, denied
	}
	var namespace struct {
		Metadata struct {
			UID, ResourceVersion string
			Labels               map[string]string
		}
	}
	raw, e = b.run(ctx, "get", "namespace", ns, "-o", "json")
	if e != nil || json.Unmarshal(raw, &namespace) != nil || ValidateSecureNamespace(namespace.Metadata.Labels, q.TenantID, "token-bound-project", record) != nil || namespace.Metadata.Labels[retiredLabel] != "" {
		return receipt, denied
	}
	binding := maintenance.Binding{TenantID: q.TenantID, NetworkID: record, TargetSHA256: r.BindingDigest(q)}
	spec, _ := json.Marshal(q.Network)
	h := sha256.Sum256(spec)
	binding.NetworkSHA256 = hex.EncodeToString(h[:])
	receipt.Binding = &binding
	receipt.Operation = o.Operation
	var bundle coldBundle
	if o.Action == "capture" {
		if _, e := os.Lstat(o.Artifact); !os.IsNotExist(e) {
			return receipt, errors.New("capture requires a new private artifact path")
		}
	}
	if o.Action == "capture" {
		if namespace.Metadata.Labels[coldLabel] != "" {
			return receipt, errors.New("cold checkpoint exists; restore/resume it before a new capture")
		}
		plan, e := r.coldInventory(ctx, q, namespace.Metadata.UID)
		if e != nil {
			return receipt, e
		}
		journal, _ := json.Marshal(plan)
		if b.apply(ctx, map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "immutable": true, "metadata": map[string]any{"name": coldJournal, "namespace": ns, "labels": map[string]string{"fabric.sathonate.io/tenant": q.TenantID, "fabric.sathonate.io/network": record}}, "data": map[string]string{"plan.json": string(journal)}}) != nil {
			return receipt, denied
		}
		namespace.Metadata.Labels[coldLabel] = plan.Digest[:60]
		if r.coldNamespacePatch(ctx, ns, namespace.Metadata.UID, namespace.Metadata.ResourceVersion, namespace.Metadata.Labels) != nil {
			return receipt, denied
		}
		if r.coldScale(ctx, plan, false) != nil {
			return receipt, errors.New("quiescence incomplete; leave API stopped and recover the recorded checkpoint")
		}
		if r.coldAllStopped(ctx, plan) != nil {
			return receipt, denied
		}
		bundle = coldBundle{Schema: "sathonate.cold-bundle/v1", Plan: plan, Control: map[string]json.RawMessage{}, Archives: map[string][]byte{}}
		tables := []struct{ name, where string }{{"sathonate_networks", "id=$2"}, {"sathonate_network_revisions", "network_id=$2"}, {"sathonate_operations", "network_id=$2"}, {"sathonate_deployments", "operation_id IN(SELECT id FROM sathonate_operations WHERE tenant_id=$1 AND network_id=$2)"}}
		for _, table := range tables {
			var data []byte
			query := "SELECT COALESCE(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text),'[]'::jsonb) FROM " + table.name + " t WHERE tenant_id=$1 AND " + table.where
			if tx.QueryRow(ctx, query, q.TenantID, record).Scan(&data) != nil {
				return receipt, denied
			}
			bundle.Control[table.name] = data
		}
		raw, e = b.run(ctx, "-n", ns, "get", "secrets,configmaps,fabriccas,fabricpeers,fabricorderernodes,fabricchaincodes", "-o", "json")
		if e != nil {
			return receipt, denied
		}
		bundle.Resources = raw
		privateBytes := len(bundle.Resources)
		for _, data := range bundle.Control {
			privateBytes += len(data)
		}
		for _, v := range plan.Volumes {
			archive, e := r.coldVolume(ctx, plan, v, nil, "capture")
			if e != nil {
				return receipt, e
			}
			privateBytes += (len(archive)*4 + 2) / 3
			if privateBytes > maintenance.MaxBackupBytes-65536 {
				return receipt, errors.New("cold bundle exceeds bounded format; use an approved storage snapshot system")
			}
			bundle.Archives[v.Name] = archive
		}
		encoded, e := json.Marshal(bundle)
		if e != nil {
			return receipt, denied
		}
		defer clear(encoded)
		encrypted, _, e := maintenance.SealBackup(binding, o.Key, encoded, time.Now().UTC())
		if e != nil {
			return receipt, errors.New("cold bundle exceeds the bounded encryption format or encryption failed; checkpoint remains frozen")
		}
		defer clear(encrypted)
		if maintenance.CreatePrivate(o.Artifact, encrypted) != nil {
			return receipt, errors.New("new private artifact could not be committed; checkpoint remains frozen")
		}
		h := sha256.Sum256(encrypted)
		receipt.ArtifactSHA256 = hex.EncodeToString(h[:])
	} else {
		// Authenticate the entire bundle before any write. Input is bounded and
		// exact original namespace/PVC UIDs are mandatory: no clone identity reuse.
		f, e := privateColdInput(o.Artifact)
		if e != nil {
			return receipt, denied
		}
		defer clear(f)
		plain, _, e := maintenance.OpenBackup(binding, o.Key, f)
		if e != nil {
			return receipt, denied
		}
		defer clear(plain)
		if json.Unmarshal(plain, &bundle) != nil || bundle.Schema != "sathonate.cold-bundle/v1" || bundle.Plan.NamespaceUID != namespace.Metadata.UID || bundle.Plan.Namespace != ns || bundle.Plan.Tenant != q.TenantID || bundle.Plan.Operation != o.Operation || bundle.Plan.TargetSHA256 != binding.TargetSHA256 || bundle.Plan.NetworkSHA256 != binding.NetworkSHA256 || coldPlanDigest(bundle.Plan) != bundle.Plan.Digest || namespace.Metadata.Labels[coldLabel] != bundle.Plan.Digest[:60] {
			return receipt, denied
		}
		for _, v := range bundle.Plan.Volumes {
			raw, e = b.run(ctx, "-n", ns, "get", "pvc", v.Name, "-o", "json")
			var claim struct {
				Metadata struct{ UID string }
				Status   struct{ Phase string }
			}
			if e != nil || json.Unmarshal(raw, &claim) != nil || claim.Metadata.UID != v.UID || claim.Status.Phase != "Bound" {
				return receipt, denied
			}
		}
		if o.Action == "restore" && r.coldAllStopped(ctx, bundle.Plan) != nil {
			return receipt, denied
		}
		if r.coldCompareResources(ctx, bundle.Plan, bundle.Resources, o.Action == "restore") != nil {
			return receipt, errors.New("identity or configuration drift prevents cold recovery")
		}
		// Control-plane rows are compared, not silently rolled back over newer
		// authorization/history. Full PostgreSQL loss requires a separate DB restore.
		if len(bundle.Control) != 4 {
			return receipt, denied
		}
		for table, data := range bundle.Control {
			where := ""
			switch table {
			case "sathonate_networks":
				where = "id=$2"
			case "sathonate_network_revisions", "sathonate_operations":
				where = "network_id=$2"
			case "sathonate_deployments":
				where = "operation_id IN(SELECT id FROM sathonate_operations WHERE tenant_id=$1 AND network_id=$2)"
			default:
				return receipt, denied
			}
			var current []byte
			if tx.QueryRow(ctx, "SELECT COALESCE(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text),'[]'::jsonb) FROM "+table+" t WHERE tenant_id=$1 AND "+where, q.TenantID, record).Scan(&current) != nil || !coldJSONEqual(data, current) {
				return receipt, errors.New("control-plane snapshot changed; database rollback is not authorized")
			}
		}
		if o.Action == "restore" {
			if len(bundle.Archives) != len(bundle.Plan.Volumes) {
				return receipt, denied
			}
			for _, v := range bundle.Plan.Volumes {
				if validateColdArchive(bundle.Archives[v.Name]) != nil {
					return receipt, denied
				}
			}
			for _, v := range bundle.Plan.Volumes {
				archive, ok := bundle.Archives[v.Name]
				if !ok {
					return receipt, denied
				}
				raw, e = b.run(ctx, "-n", ns, "get", "pvc", v.Name, "-o", "json")
				var claim struct {
					Metadata struct {
						UID, ResourceVersion string
						Annotations          map[string]string
					}
				}
				if e != nil || json.Unmarshal(raw, &claim) != nil || claim.Metadata.UID != v.UID {
					return receipt, denied
				}
				marker := maintenance.HashBytes(archive)
				mode := "restore"
				if claim.Metadata.Annotations["fabric.sathonate.io/cold-restored"] == marker {
					mode = "verify"
				}
				if _, e = r.coldVolume(ctx, bundle.Plan, v, archive, mode); e != nil {
					return receipt, e
				}
				if mode == "restore" {
					// Read a fresh version after the helper's clean unmount.
					raw, e = b.run(ctx, "-n", ns, "get", "pvc", v.Name, "-o", "json")
					if e != nil || json.Unmarshal(raw, &claim) != nil || claim.Metadata.UID != v.UID {
						return receipt, denied
					}
					if claim.Metadata.Annotations == nil {
						claim.Metadata.Annotations = map[string]string{}
					}
					claim.Metadata.Annotations["fabric.sathonate.io/cold-restored"] = marker
					patch, _ := json.Marshal([]map[string]any{{"op": "test", "path": "/metadata/uid", "value": v.UID}, {"op": "test", "path": "/metadata/resourceVersion", "value": claim.Metadata.ResourceVersion}, {"op": "add", "path": "/metadata/annotations", "value": claim.Metadata.Annotations}})
					if _, e = b.run(ctx, "-n", ns, "patch", "pvc", v.Name, "--type=json", "-p", string(patch)); e != nil {
						return receipt, errors.New("restore acknowledgement ambiguous; verify volume before repeating")
					}
				}
			}
		} else {
			if r.coldAllStopped(ctx, bundle.Plan) != nil {
				return receipt, errors.New("resume requires the exact checkpoint's writers stopped; no automatic mixed-state restart")
			}
			if len(bundle.Archives) != len(bundle.Plan.Volumes) {
				return receipt, denied
			}
			for _, v := range bundle.Plan.Volumes {
				archive := bundle.Archives[v.Name]
				if validateColdArchive(archive) != nil {
					return receipt, denied
				}
				if _, e = r.coldVolume(ctx, bundle.Plan, v, archive, "verify"); e != nil {
					return receipt, errors.New("cold data verification failed before resume")
				}
			}
			// Reopening execution is explicit. All original PVC and Deployment UIDs
			// remain required. Cold fence stays in place until every rollout is ready.
			if e = r.coldScale(ctx, bundle.Plan, true); e != nil {
				return receipt, e
			}
			delete(namespace.Metadata.Labels, coldLabel)
			raw, e = b.run(ctx, "get", "namespace", ns, "-o", "json")
			if e != nil || json.Unmarshal(raw, &namespace) != nil {
				return receipt, denied
			}
			delete(namespace.Metadata.Labels, coldLabel)
			if r.coldNamespacePatch(ctx, ns, bundle.Plan.NamespaceUID, namespace.Metadata.ResourceVersion, namespace.Metadata.Labels) != nil {
				return receipt, denied
			}
			raw, e = b.run(ctx, "-n", ns, "get", "configmap", coldJournal, "-o", "json")
			var journal resourceMetadata
			if e != nil || json.Unmarshal(raw, &journal) != nil || journal.Metadata.Labels["fabric.sathonate.io/tenant"] != q.TenantID {
				return receipt, denied
			}
			if b.deleteCleanupRef(ctx, CleanupRef{Resource: "configmaps", Namespace: ns, Name: coldJournal, UID: journal.Metadata.UID}) != nil {
				return receipt, denied
			}

		}
	}
	if tx.Commit(ctx) != nil {
		return receipt, denied
	}
	receipt.Volumes = len(bundle.Plan.Volumes)
	receipt.WorkloadsQuiesced = o.Action != "resume"
	receipt.Notice = "Local cold volume recovery only; encrypted bundle includes frozen control rows and identity resources. No production, clone activation, database rollback or full disaster recovery is qualified. Resume requires fresh API transaction verification."
	return receipt, nil
}
func coldPlanDigest(p coldPlan) string {
	p.Digest = ""
	raw, _ := json.Marshal(p)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
func coldJSONEqual(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	aa, _ := json.Marshal(x)
	bb, _ := json.Marshal(y)
	return bytes.Equal(aa, bb)
}
func (r SecureRunner) coldNamespacePatch(ctx context.Context, ns, uid, version string, labels map[string]string) error {
	patch, _ := json.Marshal([]map[string]any{{"op": "test", "path": "/metadata/uid", "value": uid}, {"op": "test", "path": "/metadata/resourceVersion", "value": version}, {"op": "add", "path": "/metadata/labels", "value": labels}})
	_, e := r.base().run(ctx, "patch", "namespace", ns, "--type=json", "-p", string(patch))
	return e
}
func (r SecureRunner) coldInventory(ctx context.Context, q Request, uid string) (coldPlan, error) {
	p := coldPlan{Schema: "sathonate.cold-plan/v1", Namespace: r.base().namespace(q.TenantID, q.NetworkID), NamespaceUID: uid, TargetSHA256: r.BindingDigest(q), Tenant: q.TenantID, Network: q.NetworkID, Operation: q.OperationID}
	spec, _ := json.Marshal(q.Network)
	h := sha256.Sum256(spec)
	p.NetworkSHA256 = hex.EncodeToString(h[:])
	b := r.base()
	raw, e := b.run(ctx, "-n", p.Namespace, "get", "deployments", "-o", "json")
	var list struct {
		Items []struct {
			Metadata struct {
				Name, Namespace, UID string
				Labels               map[string]string
			}
			Spec struct {
				Replicas int
				Template json.RawMessage
			}
		}
	}
	if e != nil || json.Unmarshal(raw, &list) != nil {
		return p, errors.New("cold deployment inventory unavailable")
	}
	names := localDeletionReleases(q.Network)
	claims := map[string]bool{}
	for _, d := range list.Items {
		var meta resourceMetadata
		meta.Metadata.Name = d.Metadata.Name
		meta.Metadata.Labels = d.Metadata.Labels
		if d.Metadata.Namespace != p.Namespace || d.Metadata.UID == "" || d.Spec.Replicas != 1 || !localDeletionWorkloadOwned(q, names, meta) {
			return p, errors.New("cold capture requires all owned deployments at their approved single replica")
		}
		var images struct {
			Spec struct{ Containers, InitContainers []struct{ Image string } }
		}
		if json.Unmarshal(d.Spec.Template, &images) != nil || len(images.Spec.Containers) != 1 {
			return p, errors.New("cold workload images invalid")
		}
		allowed := map[string]bool{SecureLocalProfile().ControllerImage: true, r.binding.config.WorkerImage: true}
		for _, image := range r.binding.config.Runtime.Images {
			allowed[image] = true
		}
		for image := range r.binding.config.Runtime.CodeArtifacts {
			allowed[image] = true
		}
		for _, container := range append(images.Spec.Containers, images.Spec.InitContainers...) {
			if !allowed[container.Image] {
				return p, errors.New("unapproved image prevents cold maintenance")
			}
		}
		hash, e := recoveryTemplateHash(d.Spec.Template)
		if e != nil {
			return p, e
		}
		p.Deployments = append(p.Deployments, coldDeployment{d.Metadata.Name, d.Metadata.UID, hash, d.Spec.Replicas})
		var t struct {
			Spec struct {
				Volumes []struct{ PersistentVolumeClaim *struct{ ClaimName string } }
			}
		}
		if json.Unmarshal(d.Spec.Template, &t) != nil {
			return p, errors.New("invalid cold workload template")
		}
		for _, v := range t.Spec.Volumes {
			if v.PersistentVolumeClaim != nil {
				claims[v.PersistentVolumeClaim.ClaimName] = true
			}
		}
	}
	if len(p.Deployments) == 0 || len(claims) == 0 {
		return p, errors.New("cold inventory empty")
	}
	sort.Slice(p.Deployments, func(i, j int) bool {
		a, b := p.Deployments[i], p.Deployments[j]
		if (a.Name == "secure-controller") != (b.Name == "secure-controller") {
			return a.Name == "secure-controller"
		}
		return a.Name < b.Name
	})
	raw, e = b.run(ctx, "-n", p.Namespace, "get", "jobs", "-o", "json")
	var jobs struct {
		Items []struct{ Status struct{ Active int } }
	}
	if e != nil || json.Unmarshal(raw, &jobs) != nil {
		return p, errors.New("job observations unavailable")
	}
	for _, j := range jobs.Items {
		if j.Status.Active != 0 {
			return p, errors.New("active writer job prevents cold capture")
		}
	}
	for name := range claims {
		raw, e = b.run(ctx, "-n", p.Namespace, "get", "pvc", name, "-o", "json")
		var v struct {
			Metadata struct{ UID string }
			Status   struct{ Phase string }
		}
		if e != nil || json.Unmarshal(raw, &v) != nil || v.Metadata.UID == "" || v.Status.Phase != "Bound" {
			return p, errors.New("cold volume must be original and bound")
		}
		p.Volumes = append(p.Volumes, coldVolume{name, v.Metadata.UID})
	}
	sort.Slice(p.Volumes, func(i, j int) bool { return p.Volumes[i].Name < p.Volumes[j].Name })
	p.Digest = coldPlanDigest(p)
	return p, nil
}
func (r SecureRunner) coldScale(ctx context.Context, p coldPlan, resume bool) error {
	deployments := append([]coldDeployment(nil), p.Deployments...)
	if resume {
		sort.SliceStable(deployments, func(i, j int) bool {
			if (deployments[i].Name == "secure-controller") != (deployments[j].Name == "secure-controller") {
				return deployments[j].Name == "secure-controller"
			}
			return deployments[i].Name < deployments[j].Name
		})
	}
	for _, d := range deployments {
		raw, e := r.base().run(ctx, "-n", p.Namespace, "get", "deployment", d.Name, "-o", "json")
		var current struct {
			Metadata struct{ UID, ResourceVersion string }
			Spec     struct{ Template json.RawMessage }
		}
		if e != nil || json.Unmarshal(raw, &current) != nil || current.Metadata.UID != d.UID {
			return errors.New("cold deployment replaced")
		}
		hash, _ := recoveryTemplateHash(current.Spec.Template)
		if hash != d.TemplateHash {
			return errors.New("cold workload template changed")
		}
		replicas := 0
		if resume {
			replicas = d.Replicas
		}
		patch, _ := json.Marshal([]map[string]any{{"op": "test", "path": "/metadata/uid", "value": d.UID}, {"op": "test", "path": "/metadata/resourceVersion", "value": current.Metadata.ResourceVersion}, {"op": "add", "path": "/spec/replicas", "value": replicas}})
		if _, e = r.base().run(ctx, "-n", p.Namespace, "patch", "deployment", d.Name, "--type=json", "-p", string(patch)); e != nil {
			return errors.New("cold scale not acknowledged")
		}
		if resume {
			if _, e = r.base().run(ctx, "-n", p.Namespace, "rollout", "status", "deployment/"+d.Name, "--timeout=180s"); e != nil {
				return errors.New("cold resume rollout incomplete")
			}
		} else {
			selector := "release=" + d.Name
			if d.Name == "secure-controller" {
				selector = "app=secure-controller"
			} else if strings.HasPrefix(d.Name, "cc-") {
				selector = "chaincode=" + d.Name
			}
			if _, e = r.base().run(ctx, "-n", p.Namespace, "wait", "--for=delete", "pod", "-l", selector, "--timeout=90s"); e != nil {
				return errors.New("cold workload shutdown not verified")
			}
		}
	}
	return nil
}
func (r SecureRunner) coldAllStopped(ctx context.Context, p coldPlan) error {
	raw, e := r.base().run(ctx, "-n", p.Namespace, "get", "deployments", "-o", "json")
	var list struct {
		Items []struct {
			Metadata struct{ Name, UID string }
			Spec     struct {
				Replicas int
				Template json.RawMessage
			}
			Status struct{ Replicas int }
		}
	}
	if e != nil || json.Unmarshal(raw, &list) != nil || len(list.Items) != len(p.Deployments) {
		return errors.New("cold workload inventory changed")
	}
	wanted := map[string]coldDeployment{}
	for _, d := range p.Deployments {
		wanted[d.Name] = d
	}
	for _, d := range list.Items {
		plan, ok := wanted[d.Metadata.Name]
		hash, _ := recoveryTemplateHash(d.Spec.Template)
		if !ok || d.Metadata.UID != plan.UID || d.Spec.Replicas != 0 || d.Status.Replicas != 0 || hash != plan.TemplateHash {
			return errors.New("all owned writers must remain stopped")
		}
	}
	raw, e = r.base().run(ctx, "-n", p.Namespace, "get", "pods", "-o", "json")
	var pods struct {
		Items []struct{ Status struct{ Phase string } }
	}
	if e != nil || json.Unmarshal(raw, &pods) != nil {
		return errors.New("cold pod inventory unavailable")
	}
	for _, pod := range pods.Items {
		if pod.Status.Phase != "Succeeded" && pod.Status.Phase != "Failed" {
			return errors.New("live or pending pod prevents consistent cold capture")
		}
	}
	return nil
}

// io/fs archives remain private binary streams; the helper has no API token,
// no host mounts and only the exact original PVC. Restore refuses nonempty data.
func (r SecureRunner) coldVolume(ctx context.Context, p coldPlan, v coldVolume, archive []byte, mode string) ([]byte, error) {
	b := r.base()
	raw, e := b.run(ctx, "-n", p.Namespace, "get", "pvc", v.Name, "-o", "json")
	var claim struct {
		Metadata struct{ UID string }
		Status   struct{ Phase string }
	}
	if e != nil || json.Unmarshal(raw, &claim) != nil || claim.Metadata.UID != v.UID || claim.Status.Phase != "Bound" {
		return nil, errors.New("cold original PVC unavailable")
	}
	name := "cold-volume-" + maintenance.HashBytes([]byte(v.Name))[:16]
	pod := map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": name, "namespace": p.Namespace}, "spec": map[string]any{"restartPolicy": "Never", "terminationGracePeriodSeconds": 1, "automountServiceAccountToken": false, "securityContext": map[string]any{"runAsUser": 0, "runAsGroup": 0, "seccompProfile": map[string]any{"type": "RuntimeDefault"}}, "containers": []any{map[string]any{"name": "archive", "image": r.binding.config.WorkerImage, "imagePullPolicy": "Never", "command": []string{"sleep", "600"}, "securityContext": map[string]any{"allowPrivilegeEscalation": false, "capabilities": map[string]any{"drop": []string{"ALL"}, "add": []string{"DAC_OVERRIDE", "CHOWN", "FOWNER"}}}, "volumeMounts": []any{map[string]any{"name": "data", "mountPath": "/volume", "readOnly": mode != "restore"}}, "resources": map[string]any{"requests": map[string]any{"cpu": "25m", "memory": "64Mi"}, "limits": map[string]any{"cpu": "500m", "memory": "512Mi"}}}}, "volumes": []any{map[string]any{"name": "data", "persistentVolumeClaim": map[string]any{"claimName": v.Name}}}}}
	encoded, _ := json.Marshal(pod)
	cmd := b.command(ctx, "-n", p.Namespace, "create", "-f", "-", "-o", "json")
	var created bytes.Buffer
	cmd.Stdout = &created
	cmd.Stdin = bytes.NewReader(encoded)
	if cmd.Run() != nil {
		return nil, errors.New("cold helper creation refused; existing helpers require inspection")
	}
	var helper resourceMetadata
	if json.Unmarshal(created.Bytes(), &helper) != nil || helper.Metadata.Name != name || helper.Metadata.Namespace != p.Namespace || helper.Metadata.UID == "" {
		return nil, errors.New("cold helper creation identity not acknowledged")
	}
	defer b.deleteCleanupRef(context.Background(), CleanupRef{Resource: "pods", Namespace: p.Namespace, Name: name, UID: helper.Metadata.UID})
	if _, e = b.run(ctx, "-n", p.Namespace, "wait", "pod/"+name, "--for=condition=Ready", "--timeout=90s"); e != nil {
		return nil, errors.New("cold helper unavailable")
	}
	script := coldCaptureScript
	if mode == "restore" {
		script = coldRestoreScript
	} else if mode == "verify" {
		script = coldVerifyScript
	} else if mode != "capture" {
		return nil, errors.New("invalid cold transfer mode")
	}
	observed, e := b.run(ctx, "-n", p.Namespace, "get", "pod", name, "-o", "json")
	var current resourceMetadata
	if e != nil || json.Unmarshal(observed, &current) != nil || current.Metadata.UID != helper.Metadata.UID {
		return nil, errors.New("cold helper was replaced; transfer refused")
	}
	cmd = b.command(ctx, "-n", p.Namespace, "exec", "-i", name, "--", "python", "-c", script)
	cmd.Stdin = bytes.NewReader(archive)
	var output cappedColdBuffer
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	if cmd.Run() != nil {
		return nil, errors.New("cold archive transfer refused; restore requires an empty original volume and a valid bounded archive")
	}
	return output.Bytes(), nil
}

type cappedColdBuffer struct{ bytes.Buffer }

func (b *cappedColdBuffer) Write(v []byte) (int, error) {
	if b.Len()+len(v) > maintenance.MaxBackupBytes {
		return 0, errors.New("archive limit")
	}
	return b.Buffer.Write(v)
}
func privateColdInput(path string) ([]byte, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, maintenance.MaxBackupBytes*2+1))
}

const coldCaptureScript = `import os,sys,tarfile,io,pathlib
os.chdir('/volume')
for base,dirs,files in os.walk('.'):
 for name in dirs+files:
  if os.path.islink(os.path.join(base,name)):raise RuntimeError('links not supported')
b=io.BytesIO()
with tarfile.open(fileobj=b,mode='w:gz') as t:t.add('.',arcname='data',filter=lambda m:None if 'lost+found' in pathlib.PurePosixPath(m.name).parts else m)
if b.tell()>16777216:raise RuntimeError('archive limit')
sys.stdout.buffer.write(b.getvalue())
`
const coldRestoreScript = `import os,sys,tarfile,io,pathlib,shutil
if any(x!='lost+found' for x in os.listdir('/volume')):raise RuntimeError('volume must be empty')
b=sys.stdin.buffer.read(16777217)
if len(b)>16777216:raise RuntimeError('archive limit')
with tarfile.open(fileobj=io.BytesIO(b),mode='r:gz') as t:
 members=t.getmembers()
 if len(members)>100000 or sum(x.size for x in members)>134217728:raise RuntimeError('expanded archive limit')
 seen=set();validated=[]
 for m in members:
  p=pathlib.PurePosixPath(m.name)
  if p.is_absolute() or '..' in p.parts or not p.parts or p.parts[0]!='data' or not (m.isdir() or m.isfile()) or m.name in seen or m.uid<0 or m.gid<0 or m.uid>2147483647 or m.gid>2147483647:raise RuntimeError('unsafe archive')
  seen.add(m.name);dest=pathlib.Path('/volume').joinpath(*p.parts[1:]);validated.append((m,dest))
 for m,dest in validated:
  if m.isdir():dest.mkdir(parents=True,exist_ok=True)
  else:
   dest.parent.mkdir(parents=True,exist_ok=True)
   with dest.open('xb') as f:shutil.copyfileobj(t.extractfile(m),f)
 for m,dest in sorted(validated,key=lambda x:len(x[1].parts),reverse=True):
  os.chown(dest,m.uid,m.gid);os.chmod(dest,m.mode&0o777);os.utime(dest,(m.mtime,m.mtime))
  fd=os.open(dest,os.O_RDONLY)
  try:os.fsync(fd)
  finally:os.close(fd)
print('RESTORED')
`

func validateColdArchive(raw []byte) error {
	bad := errors.New("invalid cold archive")
	if len(raw) == 0 || len(raw) > maintenance.MaxBackupBytes {
		return bad
	}
	gzipReader, e := gzip.NewReader(bytes.NewReader(raw))
	if e != nil {
		return bad
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	seen := map[string]bool{}
	var total int64
	for count := 0; ; count++ {
		header, e := tarReader.Next()
		if e == io.EOF {
			break
		}
		if e != nil || count >= 100000 {
			return bad
		}
		name := path.Clean(header.Name)
		if header.Name == "" || strings.HasPrefix(header.Name, "/") || name == ".." || strings.HasPrefix(name, "../") || (name != "data" && !strings.HasPrefix(name, "data/")) || strings.Contains(header.Name, "\\") || seen[name] || (header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeDir) || header.Size < 0 || header.Uid < 0 || header.Gid < 0 || header.Uid > 2147483647 || header.Gid > 2147483647 {
			return bad
		}
		seen[name] = true
		total += header.Size
		if total > 128<<20 {
			return bad
		}
		if _, e = io.Copy(io.Discard, tarReader); e != nil {
			return bad
		}
	}
	if !seen["data"] {
		return bad
	}
	return nil
}

func (r SecureRunner) coldCompareResources(ctx context.Context, p coldPlan, encoded []byte, restore bool) error {
	var objects struct{ Items []json.RawMessage }
	if json.Unmarshal(encoded, &objects) != nil || len(objects.Items) == 0 {
		return errors.New("invalid cold identity inventory")
	}
	resources := map[string]string{"Secret": "secrets", "ConfigMap": "configmaps", "FabricCA": "fabriccas", "FabricPeer": "fabricpeers", "FabricOrdererNode": "fabricorderernodes", "FabricChaincode": "fabricchaincodes"}
	// Validate original CR/config ownership before recreating missing Secrets.
	sort.SliceStable(objects.Items, func(i, j int) bool {
		var a, b struct{ Kind string }
		json.Unmarshal(objects.Items[i], &a)
		json.Unmarshal(objects.Items[j], &b)
		return a.Kind != "Secret" && b.Kind == "Secret"
	})
	for _, rawItem := range objects.Items {
		var item struct {
			Kind                   string
			Metadata               struct{ Name, Namespace, UID string }
			Data, BinaryData, Spec json.RawMessage
		}
		if json.Unmarshal(rawItem, &item) != nil {
			return errors.New("invalid cold resource record")
		}
		resource, ok := resources[item.Kind]
		if !ok || item.Metadata.Namespace != p.Namespace || !safeCleanupName(item.Metadata.Name) || item.Metadata.UID == "" {
			return errors.New("invalid cold resource binding")
		}
		raw, e := r.base().run(ctx, "-n", p.Namespace, "get", resource, item.Metadata.Name, "--ignore-not-found", "-o", "json")
		if e != nil {
			return errors.New("cold identity inspection unavailable")
		}
		if len(bytes.TrimSpace(raw)) == 0 && restore && item.Kind == "Secret" {
			var secret map[string]any
			if json.Unmarshal(rawItem, &secret) != nil {
				return errors.New("invalid encrypted Secret")
			}
			metadata := secret["metadata"].(map[string]any)
			for _, key := range []string{"uid", "resourceVersion", "managedFields", "creationTimestamp", "deletionTimestamp", "deletionGracePeriodSeconds", "generateName"} {
				delete(metadata, key)
			}
			annotations, ok := metadata["annotations"].(map[string]any)
			if !ok {
				annotations = map[string]any{}
			}
			annotations["fabric.sathonate.io/cold-restored-from"] = item.Metadata.UID
			annotations["fabric.sathonate.io/cold-plan"] = p.Digest
			metadata["annotations"] = annotations
			encoded, _ := json.Marshal(secret)
			cmd := r.base().command(ctx, "-n", p.Namespace, "create", "-f", "-")
			cmd.Stdin = bytes.NewReader(encoded)
			if cmd.Run() != nil {
				return errors.New("missing identity create not acknowledged; inspect checkpoint before retry")
			}
			raw, e = r.base().run(ctx, "-n", p.Namespace, "get", resource, item.Metadata.Name, "-o", "json")
			if e != nil {
				return errors.New("restored identity observation unavailable")
			}
		}
		var current struct {
			Metadata struct {
				UID         string
				Annotations map[string]string
			}
			Data, BinaryData, Spec json.RawMessage
		}
		if json.Unmarshal(raw, &current) != nil {
			return errors.New("cold identity resource missing")
		}
		sameUID := current.Metadata.UID == item.Metadata.UID
		restoredUID := item.Kind == "Secret" && current.Metadata.UID != "" && current.Metadata.Annotations["fabric.sathonate.io/cold-restored-from"] == item.Metadata.UID && current.Metadata.Annotations["fabric.sathonate.io/cold-plan"] == p.Digest
		if !sameUID && !restoredUID {
			return errors.New("cold identity resource replaced outside approved restore")
		}
		equal := func(a, b []byte) bool {
			if len(a) == 0 || len(b) == 0 {
				return len(a) == len(b)
			}
			return coldJSONEqual(a, b)
		}
		if !equal(item.Data, current.Data) || !equal(item.BinaryData, current.BinaryData) || !equal(item.Spec, current.Spec) {
			return errors.New("cold identity resource changed")
		}
	}
	return nil
}

func (r SecureRunner) restoreColdControl(ctx context.Context, tx pgx.Tx, o ColdOptions) error {
	denied := errors.New("scoped control-plane restore authentication or ownership failed")
	binding := o.Binding
	if binding == nil || binding.TenantID != o.Tenant || binding.TargetSHA256 != r.BindingDigest(Request{}) {
		return denied
	}
	encoded, e := privateColdInput(o.Artifact)
	if e != nil {
		return denied
	}
	defer clear(encoded)
	plain, _, e := maintenance.OpenBackup(*binding, o.Key, encoded)
	if e != nil {
		return denied
	}
	defer clear(plain)
	var bundle coldBundle
	if json.Unmarshal(plain, &bundle) != nil || bundle.Schema != "sathonate.cold-bundle/v1" || len(bundle.Control) != 4 || bundle.Plan.Tenant != o.Tenant || bundle.Plan.Network != binding.NetworkID || bundle.Plan.Operation != o.Operation || bundle.Plan.NetworkSHA256 != binding.NetworkSHA256 || bundle.Plan.TargetSHA256 != binding.TargetSHA256 || coldPlanDigest(bundle.Plan) != bundle.Plan.Digest {
		return denied
	}
	// Authenticate the original live target before importing even scoped rows.
	q := Request{TenantID: o.Tenant, NetworkID: binding.NetworkID}
	ns := r.base().namespace(q.TenantID, q.NetworkID)
	if ns != bundle.Plan.Namespace || r.base().validateSecure(ctx) != nil {
		return denied
	}
	raw, e := r.base().run(ctx, "get", "namespace", ns, "-o", "json")
	var namespace struct {
		Metadata struct {
			UID    string
			Labels map[string]string
		}
	}
	if e != nil || json.Unmarshal(raw, &namespace) != nil || namespace.Metadata.UID != bundle.Plan.NamespaceUID || ValidateSecureNamespace(namespace.Metadata.Labels, q.TenantID, "token-bound-project", q.NetworkID) != nil || namespace.Metadata.Labels[coldLabel] != bundle.Plan.Digest[:60] {
		return denied
	}
	operationIDs := map[string]bool{}
	var operationsRows []map[string]json.RawMessage
	if json.Unmarshal(bundle.Control["sathonate_operations"], &operationsRows) != nil || len(operationsRows) == 0 {
		return denied
	}
	text := func(v json.RawMessage) string { var s string; json.Unmarshal(v, &s); return s }
	for _, row := range operationsRows {
		if text(row["tenant_id"]) != o.Tenant || text(row["network_id"]) != binding.NetworkID {
			return denied
		}
		operationIDs[text(row["id"])] = true
	}
	for _, table := range []string{"sathonate_networks", "sathonate_network_revisions", "sathonate_operations", "sathonate_deployments"} {
		var rows []map[string]json.RawMessage
		data, ok := bundle.Control[table]
		if !ok || json.Unmarshal(data, &rows) != nil || len(rows) == 0 {
			return denied
		}
		for _, row := range rows {
			if text(row["tenant_id"]) != o.Tenant {
				return denied
			}
			switch table {
			case "sathonate_networks":
				if text(row["id"]) != binding.NetworkID {
					return denied
				}
			case "sathonate_network_revisions", "sathonate_operations":
				if text(row["network_id"]) != binding.NetworkID {
					return denied
				}
			case "sathonate_deployments":
				if !operationIDs[text(row["operation_id"])] {
					return denied
				}
			}
		}
		if _, e = tx.Exec(ctx, "INSERT INTO "+table+" SELECT * FROM jsonb_populate_recordset(NULL::"+table+",$1::jsonb) ON CONFLICT DO NOTHING", data); e != nil {
			return denied
		}
	}
	return nil
}

const coldVerifyScript = `import os,sys,tarfile,io,pathlib,hashlib
b=sys.stdin.buffer.read(16777217)
if len(b)>16777216:raise RuntimeError('archive limit')
expected=set()
with tarfile.open(fileobj=io.BytesIO(b),mode='r:gz') as t:
 for m in t.getmembers():
  p=pathlib.PurePosixPath(m.name)
  if p.is_absolute() or '..' in p.parts or not p.parts or p.parts[0]!='data' or not (m.isdir() or m.isfile()):raise RuntimeError('unsafe archive')
  dest=pathlib.Path('/volume').joinpath(*p.parts[1:]);expected.add(dest)
  if dest.is_symlink() or not dest.exists():raise RuntimeError('restored path changed')
  stat=dest.stat()
  if stat.st_uid!=m.uid or stat.st_gid!=m.gid or (stat.st_mode&0o777)!=(m.mode&0o777):raise RuntimeError('restored ownership changed')
  if m.isfile():
   if not dest.is_file() or stat.st_size!=m.size:raise RuntimeError('restored file changed')
   with dest.open('rb') as f:
    if hashlib.file_digest(f,'sha256').digest()!=hashlib.file_digest(t.extractfile(m),'sha256').digest():raise RuntimeError('restored data changed')
actual=set(pathlib.Path('/volume').rglob('*'))
actual={x for x in actual if 'lost+found' not in x.parts};actual.add(pathlib.Path('/volume'))
if actual!=expected:raise RuntimeError('restored file inventory changed')
print('VERIFIED')
`

// ColdDatabaseAllowed restricts this qualification utility to the existing
// disposable local test database. It must not contact or lock production DBs.
func ColdDatabaseAllowed(host, database string) bool {
	ip := net.ParseIP(host)
	return database == "fabric_smp_tests" && (strings.EqualFold(host, "localhost") || (ip != nil && ip.IsLoopback()))
}
