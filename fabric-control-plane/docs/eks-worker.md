# EKS worker and qualification

This adapter deploys Fabric into an **existing** approved cluster. It does not create an EKS cluster, VPC, node group, EBS driver, registry, IAM role, DNS zone or load balancer. Production mode and `--enable-cloud-execution` remain closed. `--deployment-mode eks-qualification` is a separate loopback-only administrator qualification path, requiring PostgreSQL, an exact approved target and a live preflight for every run.

## Implementation

The adapter reuses `KubernetesRunner` Job observation and `kubernetesfabric.Worker` CA → orderers → peers → channels → transaction stages. Bevel operator v1.14.0 source is pinned to `cd236ccc61d1d159b3dadb4625d08bb5682a8451`; its controller reconciles the Fabric Helm charts. The legacy Ansible/Flux exporter is retained for historical candidates but **is not executed** for operator targets. Ansible is used by the existing local operator bootstrap; it is not inserted as a redundant second Fabric deployment engine.

The supported initial execution profile is deliberately bounded: Fabric 2.5.15, CA 1.5.15, V2_0, two organizations with one peer each, three Raft orderers in one ordering organization, one channel and the `smp_smoke` contract. Arbitrary topologies, BFT, multiple channels and arbitrary customer chaincode are rejected by this worker. Three cloud orderers require three approved availability zones. Placement maps are translated to required node affinity: this avoids the pinned upstream CRD/Helm mismatch in its `nodeSelector` field while keeping placement mandatory. The current Kubernetes test matrix is exactly minor 1.34; actual EKS compatibility is unverified until qualification.

Network JSON supplies desired topology, not credentials or commands. Administrator configuration supplies the tenant/target/namespace binding, role-backed profile, kubeconfig reference, encrypted StorageClass, image digests and node selectors. A target is bound to the exact validated network digest. An approval is bound to the entire target digest and expires within seven days; changing configuration invalidates it. Never change target bindings while an operation is active.

## Windows build and local tests

Prerequisites: PowerShell 7, Git, Go matching `go.mod`, Python 3.12+, Docker Desktop Linux engine, kubectl, kind and the existing local Bevel environment. Run from `E:\Sathonate\fabric-control-plane`. The local tests need additional free memory for a temporary network alongside any retained network; approximately 8–12 GiB allocated to Docker is preferable. Existing lower-memory hosts may need to stop other workloads first.

```powershell
$env:GOCACHE='E:\Sathonate\.cache\go-build'
$env:GOPATH='E:\Sathonate\.cache\go'
go test ./...
go vet ./...

# Build Linux worker; restore GOOS afterward for Windows Go tests.
$env:GOOS='linux'; $env:GOARCH='amd64'; $env:CGO_ENABLED='0'
go build -o bin/fabric-worker ./cmd/fabric-worker
Remove-Item Env:GOOS,Env:GOARCH,Env:CGO_ENABLED
python tools/kubernetes/build-scoped-cli.py --source E:\Sathonate\dependencies\bevel-operator-fabric --build-dir E:\Sathonate\.cache\bevel-scoped-cli --output E:\Sathonate\fabric-control-plane\bin\kubectl-hlf-scoped
docker build --target worker -f tools/kubernetes/Dockerfile -t sathonate/fabric-worker:eks-local .
docker build -f tools/kubernetes/Dockerfile.eks -t sathonate/fabric-worker:eks-scoped .
& E:\Sathonate\tools\kind.exe load docker-image sathonate/fabric-worker:eks-scoped --name smp-qualification
$env:SMP_EKS_LOCAL_E2E='1'
$env:SMP_TEST_KUBECONFIG='E:\Sathonate\.cache\smp-qualification.kubeconfig'
go test ./internal/deployment -run 'TestEKSWorkerLifecycleOnKind|TestLocalWorkerRegressionOnKind' -count=1 -v -timeout 35m
Remove-Item Env:SMP_EKS_LOCAL_E2E
```

The EKS-shaped test uses real Kubernetes RBAC, actual pinned Fabric images, channel brokering and a committed transaction. It does not call EKS preflight or AWS; it is **not** EKS evidence. Its worker image tag and single-node placement are local test accommodations. The local regression test exercises the original local adapter. Tests remove only their own namespaces and channel resources. Do not run the two real tests concurrently on a memory-constrained workstation.

PostgreSQL integration tests require `SATHONATE_TEST_DATABASE_URL` pointing to a dedicated disposable database with the operations schema. Inject that connection string from your local secret facility, never commit or print it:

```powershell
go test ./internal/persistence ./internal/deployment ./internal/operations -count=1
```

Without that variable, the PostgreSQL cases explicitly skip; mocks are not database evidence. AWS API unit tests simulate account, endpoint and permission responses and are labelled accordingly.

## Why a scoped Bevel CLI build

The selected upstream CLI performs cluster-wide CA discovery even when a namespace is supplied. It also tries public-node discovery for ClusterIP services. Granting the worker cluster-wide access would expose other organizations' CA configuration. `build-scoped-cli.py` archives the exact pinned commit into a separate build directory and applies narrowly scoped discovery/internal-URL changes; it never modifies the reference checkout. `SMP_WORKER_NAMESPACE` is set only for approved runtime execution. All Fabric commands and cryptography remain upstream. Namespace RBAC is authoritative and denies foreign reads regardless of CLI arguments. Review this patch and the resulting image provenance before cloud approval.

The EKS worker can manage Fabric CRs and Secrets only inside its approved namespace. Its cluster role permits only get/list/watch of three exact channel CR names (list/watch requests must include a matching name field selector). It cannot create cluster channels, mutate RBAC, or read another namespace. The controller validates channel requests, including namespace, wallet, host and identity references, then writes cluster-scoped channel CRs. Broker payloads containing private keys are rejected and are never recorded as progress logs. The trusted controller requires cluster-scoped channel-create and limited RBAC-bootstrap permissions; it is not an unprivileged tenant worker. A reviewed controller role, namespace network policies, quotas and admission controls are prerequisites for shared-cluster qualification.

## First authorized EKS qualification

1. Supply an existing active Kubernetes 1.34 EKS cluster, three ready/schedulable AZ placements, working DNS between workloads, encrypted EBS CSI StorageClass with delayed volume binding, and outbound access to approved registries. Actual PVC attachment and AZ failure recovery remain acceptance tests, not preflight claims.
2. Install the pinned Bevel operator/CRDs using an administrator-reviewed copy of the existing bootstrap procedure. The chart is `chart/hlf-operator`, not `charts/hlf-operator`. Preserve CRDs outside Helm release ownership as the local bootstrap does. Do not point `bootstrap-operator.ps1` at AWS: it intentionally accepts only kind. Pin the operator image by digest and annotate its Deployment with `fabric.sathonate.io/bevel-commit=cd236ccc61d1d159b3dadb4625d08bb5682a8451`. Preflight verifies that annotation, image and availability; annotation alone does not prove source provenance.
3. Build/publish the scoped worker and smoke images to an approved registry and record their manifest digests. For a reproducible EKS worker build, set `--build-arg WORKER_BASE_IMAGE=<approved-base@sha256:digest>` for `Dockerfile.eks`. Registry publishing and AWS infrastructure setup are separate authorized operations; none is performed automatically here.
4. Configure a narrowly scoped role trusted by your company SSO/workload identity, with `eks:DescribeCluster` for the existing cluster and the reviewed EKS access/RBAC mapping. No EC2/EKS creation permissions are needed by this adapter. The AWS CLI profile must resolve to this assumed role, not a root or IAM user identity:

```powershell
aws configure sso --profile sathonate-source
aws configure set role_arn <approved-role-arn> --profile sathonate-fabric-role
aws configure set source_profile sathonate-source --profile sathonate-fabric-role
aws sso login --profile sathonate-source
aws sts get-caller-identity --profile sathonate-fabric-role
# Output directory must already exist and be private.
./scripts/write-eks-kubeconfig.ps1 -AwsProfile sathonate-fabric-role -Region <region> -Cluster <existing-cluster> -Context sathonate-eks -AccountID <account-id> -RoleARN <approved-role-arn> -Output E:\Sathonate\private\eks.kubeconfig
```

5. Save a valid design and note its **draft record ID**, tenant and target ID. Copy `examples/eks-operator-targets.template.json` outside source control and replace every placeholder. The sample network is `examples/eks-qualification-network.json`. Never put credentials in either file. Inspect hashes without cloud calls:

```powershell
go build -o bin/fabric-eks.exe ./cmd/fabric-eks
./bin/fabric-eks.exe -action inspect -registry <private-targets.json> -network <saved-network.json> -tenant <tenant> -network-id <draft-record-id> -target company-eks
```

Set `runnerNamespace` and `operator.networkSHA256` to the returned values; set actual account, role, profile, cluster, context, AZ selectors, images and storage. An administrator must precreate this namespace, exclusively for that network, with labels `fabric.sathonate.io/tenant=<tenant>`, `fabric.sathonate.io/target=company-eks`, and `fabric.sathonate.io/network=<draft-record-id>`. Apply reviewed namespace isolation/quota policies separately. Registering two bindings to the same account/cluster/namespace is rejected.

6. Re-run inspect after all target edits to obtain the final `targetSHA256`, then perform read-only checks:

```powershell
./bin/fabric-eks.exe -action preflight -registry <private-targets.json> -network <saved-network.json> -tenant <tenant> -network-id <draft-record-id> -target company-eks
```

Preflight verifies assumed role, exact EKS ARN/endpoint/CA, strict AWS-only kubeconfig exec identity, server minor, namespace labels, storage/CSI registration, operator/CRDs, required permissions and ready node placements. It does not provision dependencies. Inspect/preflight never creates an approval.

7. Only after explicit owner authorization for Fabric workloads and their storage charges, create a private administrator-owned approval file (JSON array):

```json
[{"purpose":"eks-qualification","targetSHA256":"<final hash>","authorizationRef":"<approved-change-id>","expiresAt":"<UTC timestamp within seven days>"}]
```

8. Use a separate qualification PostgreSQL database, migrate it with the existing API migration command, and inject `DATABASE_URL` securely. Start the loopback qualification API. Do not share its database with the already-running local executor; a database session lock enforces one active scheduler:

```powershell
go build -o bin/fabric-api-eks.exe ./cmd/fabric-api
./bin/fabric-api-eks.exe -migrate
./bin/fabric-api-eks.exe -addr 127.0.0.1:8082 -mode local-development -deployment-mode eks-qualification -cloud-config <private-targets.json> -eks-approval <private-approvals.json>
```

The local-development principal is `local-development`; target bindings must match it. The global Cloud readiness screen continues to describe production setup; qualification uses the per-target preflight and approval checks, independently of those global environment fields. This administrator qualification service must stay loopback-only. It is not an enterprise public endpoint or a substitute for real Okta authorization tests.

9. Submit the saved revision with the existing `POST /api/v1/networks/{id}/deploy`, `If-Match`, and `Idempotency-Key` contract. Observe existing operation/events/deployment endpoints. `POST /api/v1/deployments/{operationId}/cancel` durably requests cancellation and confirms Job termination. `DELETE /api/v1/deployments/{operationId}` removes Fabric resources while retaining the approved namespace, enrollment Secrets and PVCs; a separate administrator retention decision governs data destruction. Never equate cleanup with secure erasure.

## Scheduling, retries and failure semantics

PostgreSQL atomically records cloud operations and deployment input before dispatch. A single advisory-lock owner schedules jobs; a two-second recovery loop discovers committed pending work, and lock-session failure cancels its command contexts. On API restart, running cloud operations adopt the same deterministic immutable Job and replay observed stage events. A completed Job supplies historical transaction evidence; the adapter checks current workload readiness before marking its recovered deployment ready. They do not replace its pod template. This is a single-active scheduler, not a horizontally distributed worker fleet.

Retries cancel the old Job, then use a new operation Job against the same network namespace and retained Fabric identities/resources. The target/network digests cannot drift. Unique active-operation and idempotency indexes prevent duplicate submissions. A process shutdown leaves the cloud Job for adoption; explicit cancellation persists its intent before terminating the Job and survives a subsequent restart. Worker and Job deadlines bound execution; failures keep resources for diagnosis/reconciliation. Namespace/API failures never authorize credential rotation. No raw AWS command diagnostics are exposed.

## Still unverified before production release

Actual EKS/IAM execution, private-registry pulls, EBS attachment/recovery, three-AZ behavior, network-policy enforcement, backup restoration, certificate renewal, live Okta tenant isolation and operational load. Real acceptance must verify CA TLS/enrollment, three active Raft consenters, channel membership, a committed write/query on both peers, process/worker restart recovery and cleanup/data retention. Production remains disabled until that evidence and a reviewed release decision exist.

Enrollment credential handling and residual inline operator fields are documented in [credential security](enrollment-credential-security.md). Rebuild the protected CLI and both worker images; old binaries are not a secure fallback.

## Enterprise maintenance changes

Cloud cleanup now requires the administrator-only `cleanup-preview` API and `X-Sathonate-Cleanup-Approval` digest. Broad cloud cleanup is disabled; unlabelled legacy objects fail closed. Exact UID-precondition deletion excludes namespace/Secret/PVC destruction and reports retention as unverified. See `backup-retention-cleanup.md`. Rotation planning is durable but the live mutation driver is disabled; see `credential-rotation-recovery.md`. The staged opt-in EKS harness/runbook is `eks-qualification.md`; it does not certify all acceptance gates or authorize AWS by itself.
