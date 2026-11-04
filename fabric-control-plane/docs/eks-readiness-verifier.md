# Read-only EKS readiness verifier

Run from the reviewed `E:\Sathonate\fabric-control-plane` checkout with Go 1.26, AWS CLI, kubectl, a fully populated private target registry and the exact validated saved network document. No dependencies are installed and no workload, IAM or AWS resource is changed. Existing deployment authorization and production gates are unchanged.

```powershell
go build -o bin/fabric-eks.exe ./cmd/fabric-eks
./bin/fabric-eks.exe -action readiness `
  -registry <private-targets.json> -network <saved-network.json> `
  -tenant <tenant> -network-id <draft-record-id> -target company-eks `
  -evidence <new-private-evidence.json>
```

Use a private output directory with administrator-controlled Windows ACLs. The exporter refuses to overwrite an existing file. Exit 1 means invalid input/output; exit 2 means BLOCKED prerequisites; exit 3 means NOT VERIFIED runtime checks remain. This milestone intentionally never returns a successful EKS qualification result. It does not require a deployment approval because it cannot execute workloads. Registry ownership and existing identity/cluster authorization checks still apply.

The manifest records UTC timestamps, checkout revision/cleanliness, target/network hashes, six fixed image digest fields and fixed validation messages. Source metadata describes the current checkout, not an attested binary build. Missing/uncommitted source is NOT VERIFIED. No raw Kubernetes/AWS responses, errors, object names, kubeconfig, connection strings, image repository URLs, enrollment material or user configuration are exported. Unknown runtime image map keys are never exported.

Checks cover exact supported network translation, unique supplied-registry binding, all ownership labels, an empty fresh-network workload/PVC/Fabric inventory, positive unscoped CPU/memory/pod/PVC/storage quotas, namespace-wide ingress/egress default-deny objects, pinned restricted Pod Security labels, and ready uncordoned Linux amd64 nodes without NoSchedule/NoExecute taints for every approved selector. Existing preflight checks bound assumed-role identity, endpoint/CA, Kubernetes 1.34, encrypted delayed-binding EBS CSI storage, operator pins and required controller permissions. Placeholder zero image digests are rejected.

This is a conservative fresh-network verifier, not a reconciler for an existing deployed network. Exclusivity is established only relative to the supplied authoritative registry and inspected inventory; namespace access/RBAC review remains NOT VERIFIED. Quota presence does not prove available capacity. Node eligibility does not prove actual scheduling. Default-deny presence does not prove isolation: Kubernetes allow policies are additive and CNI enforcement requires live probes. Restricted Pod Security labels do not prove admission exemptions/webhooks or Fabric template compatibility. A restricted baseline may reject current upstream Fabric templates; review compatibility without weakening enforcement automatically. Custom admission engines are not evaluated and remain NOT VERIFIED.

Private image authentication must be configured separately; this verifier checks immutable references, not registry access, signatures or image provenance. EBS IAM/KMS access, volume attachment, AZ failure recovery, live tenant denial, Fabric transactions and cleanup/retention require separate authorized qualification. Overall AWS readiness, EKS qualification and production execution remain false even when all static checks pass.

## Tests and remaining security blockers

Unit tests use explicitly simulated AWS/Kubernetes responses. They cover prerequisite success, missing quotas/policies, admission/ownership failures, occupied namespaces, taints, duplicate bindings, mutable images and secret-bearing diagnostic rejection. They are not EKS evidence. Run `go test ./... -count=1` and `go vet ./...`; PostgreSQL and real kind cases require their existing opt-ins and otherwise skip. No deployment tests should be triggered by the readiness command.

Unchanged blockers: enrollment passwords appear in CLI arguments/rendered Fabric CRs and are reused per organization; controller cluster-channel/RBAC permissions need review; effective namespace isolation/admission and foreign access remain unverified; cleanup deletes all Fabric CRs in the exclusive namespace and retains sensitive Secrets/PVCs and worker RBAC. None is repaired or waived by this verifier.

## Verification for this milestone (22 October 2026)

Executed `go test ./... -count=1`: all enabled packages passed. Executed `go vet ./...`: passed. Executed `go build -o bin/fabric-eks-readiness.exe ./cmd/fabric-eks`: passed. Focused readiness unit tests passed for simulated prerequisites, policy/ownership/quota/admission/taint failures, redaction and fail-before-cloud behavior. An explicit verbose opt-in check confirmed PostgreSQL integration and kind deployment cases skipped because their environment variables were not supplied; neither was rerun or claimed verified for this change.

The CLI was exercised with the repository placeholder target/network and a new evidence output path. It exited 2 (BLOCKED) on binding validation before AWS access, with readiness/qualification/production booleans false. No real AWS/EKS checks or deployment were performed. Source revision is unavailable in this uncommitted checkout and is explicitly NOT VERIFIED.

## Additional enterprise checks

`-check-postgres` explicitly probes `DATABASE_URL` from the local secret facility; it requires hostname-verified TLS with no plaintext fallback, a non-superuser/non-bypass role, required schema and forced tenant RLS on network/revision/maintenance tables. It never exports diagnostics or the connection string. Operations/deployments still rely on trusted controller query scoping and remain a separate NOT VERIFIED isolation gate. Standard EBS CSI controller replica availability is read when permitted; driver IAM/KMS, actual encryption/attachment, registry pulls/architecture, effective admission/CNI and certificate/controller containment remain NOT VERIFIED. No unit test contacts AWS. See `eks-qualification.md` for explicitly authorized staged acceptance commands.
