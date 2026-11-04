# Enrollment credential security milestone

## Flow and bounded design

The API accepts a strictly decoded desired network, not enrollment passwords. PostgreSQL records that specification, operations and redacted events. The Kubernetes worker generates credentials inside the approved namespace; it does not send them back to the API. Previously a single organization password was reused for registrar, peers, orderers, administrator and chaincode identities, passed in `kubectl-hlf` argv, and copied into Bevel CRs. The older Docker CA profile similarly used bootstrap arguments, credential-bearing URLs and predictable identity passwords.

The source-built pinned Bevel CLI now accepts bounded JSON logical arguments through an anonymous stdin pipe. Kernel process argv contains only `--smp-stdin-arguments`, and no passwords are placed in environment variables. The worker checks a transport capability before provisioning and fails closed with an older CLI. No fallback to password flags is permitted. Upstream Fabric/Bevel cryptography remains unchanged. The protected CLI silences upstream request logs and Cobra usage/errors, returning only fixed semantic failure markers; the worker captures output and only exports validated events/results. The pinned upstream `ca enroll` file permission is reduced from 0644 to 0600.

New Kubernetes identities use separate cryptographically random, persisted credentials: two registrars, five node identities, two administrators and the required chaincode identity (ten total in the supported profile). Node IDs are distinct, including three distinct orderer enrollment identities. Credentials are saved before registration, reused on retries, never rotated because an API read failed, and new Secrets are immutable. Registrar Secrets retain their existing names for compatibility; v2 identity Secret names are hashed by organization/identity. Existing Fabric nodes and CA databases are not silently rotated. Existing wallet certificates may be adopted; replacing a retained legacy network requires an explicit migration/rotation plan.

The Docker profile uses private JSON client configuration, a read-only per-command delivery mount, a writable tmpfs copy for Fabric CA's mandatory config rewrite, explicit writable MSP paths and deferred temporary-directory deletion. CA bootstrap configuration replaces `-b user:password` in Compose/container arguments. Its persisted state stores independently random identity passwords rather than deriving them from the registrar password. Runtime directories are protected using a current-user-only inherited DACL on Windows and 0700 on Unix; configuration files use 0600. Existing credential-bearing files/container metadata from older deployments are not retroactively erased by this change.

Bootstrap registrar privileges for new CAs are narrowed to the supported peer/orderer/admin/client registration types, with delegation, revocation, intermediate CA, CRL and affiliation management disabled. Worker RBAC remains namespace-scoped as before; controller RBAC is not broadened.

## Deliberate residual plaintext and key exposure

This is a reduction of exposure, not complete secret-reference support. The pinned Bevel CA registry uses `spec.ca.registry.identities[].pass` and its TLS counterpart. Its peer Kubernetes-store validator requires `spec.secret.enrollment.component.enrollsecret`; node TLS enrollment and chaincode credentials also use inline values. No supported enrollment-password Secret reference exists in these fields. Current CA-backed reconciliation therefore still persists credentials in these CRs and potentially operator/Helm-owned configuration, backups and audit trails. Rendering is memory-only and applied through stdin; no generated YAML is written as an artifact. A validation guard rejects known credentials outside the narrowly allowed CA/peer/orderer enrollment fields.

Bevel exposes external certificate Secret references, but adopting them requires a separately verified pre-enrollment/renewal design across peers, orderers and chaincode, plus controller validation compatibility. This milestone does not claim that conversion has happened. It does not replace a password with a fictitious Secret reference or clear required fields after reconciliation. Treat Fabric CR read privileges as secret access until that migration is qualified.

Required Kubernetes Secrets contain enrollment passwords and wallet private keys; base64 is not encryption. Require encrypted etcd/backups, restricted Secret/CR/audit access and scoped operator permissions. The worker necessarily holds plaintext in process memory and private pipes. Host/container administrators, privileged observability agents and a compromised controller remain trusted boundaries. Upstream operator logging has not been globally replaced; review/redact controller logs and restrict access before production. Workers track raw/base64 credential values for diagnostic redaction and scrub enrollment field names at API/log boundaries.

## Rotation and cleanup

Do not delete immutable credential Secrets to rotate a live identity. That loses reconciliation state without updating the CA database or existing CRs. Use an authorized maintenance operation: inventory CA/identity/certificate/CR/Secret/Helm references privately, create replacement identity credentials, enroll and verify new certificates, update supported consumers in a controlled order, verify channel/transaction behavior, revoke retired certificates with a separately authorized revoker and retire old enrollment identities. Back up and test recovery first. Automatic rotation and renewal are not implemented here.

Worker identity/profile files are removed from the memory-backed workspace on normal completion/failure; a killed worker may retain them until its pod/emptyDir is removed. Go strings are not guaranteed securely zeroed. Docker client input is removed after each call; host files/backups and interrupted-process leftovers still require a retention policy. This is not secure erasure.

EKS cleanup continues to remove all Fabric CRs in the exclusive approved namespace while retaining PVCs, enrollment/wallet Secrets and worker RBAC. Retention is intentional for diagnosis/recovery and requires a separate administrator data-destruction decision. No cloud resources were created and production execution remains disabled.

## Build and test

Build the protected CLI before either worker image; `scripts/setup-local.ps1` now does this explicitly. Use `tools/kubernetes/build-scoped-cli.py` against pinned Bevel commit `cd236ccc61d1d159b3dadb4625d08bb5682a8451`, then build the Linux worker and Docker images as in `docs/eks-worker.md`. Old worker images must be rebuilt; this source change does not modify running containers.

Run `go test ./... -count=1` and `go vet ./...`. Real infrastructure tests are separately opted in:

```powershell
$env:SMP_CREDENTIAL_DOCKER_E2E='1'
go test ./internal/localfabric -run '^TestProtectedCAConfigOnDocker$' -count=1 -v -timeout 3m
$env:SMP_EKS_LOCAL_E2E='1'
$env:SMP_TEST_KUBECONFIG='E:\Sathonate\.cache\smp-qualification.kubeconfig'
go test ./internal/deployment -run 'TestEKSWorkerLifecycleOnKind|TestLocalWorkerRegressionOnKind' -count=1 -v -timeout 35m
```

The kind tests inspect ten distinct credentials in memory and assert they are absent from Job manifests/logs; they never print Secret payloads. These are local tests, not AWS/EKS qualification. See the final execution results below for what actually ran.

## Execution results â€” 22 October 2026

Status: source changes implemented and enabled unit/static checks pass; final infrastructure regression is BLOCKED. This report does not declare the credential issue fully resolved or EKS qualified.

- `go test ./... -count=1`: passed on final source. Existing opt-in PostgreSQL/kind/Docker integration cases skip in this command; no database integration success is claimed for this milestone.
- `go vet ./...`: passed.
- Source-built protected Bevel CLI from the pinned commit: compiled successfully. Final Linux `fabric-worker` Go binary: compiled successfully.
- Intermediate worker Docker builds and kind image load succeeded before the Docker engine stopped. The final source includes later fixes and must be rebuilt into an image before acceptance; the intermediate image is not a final verified release.
- Security regression tests cover private stdin arguments, bounded input, separate identity names, narrow rendered-resource credential exceptions, raw/base64 event redaction, API error redaction, evidence allowlisting, protected Docker client input, Compose argument confidentiality, Windows directory protection and reduced registrar privileges. Final logs: `.logs/credentials-security-tests-final.log` and `.logs/credentials-go-tests-final.log`.
- `TestEKSWorkerLifecycleOnKind`: FAILED on an intermediate build after reaching real CA/orderer/peer/channel stages. The protected CLI reduced a missing chaincode approval to `sequence: '0'`, but the worker expected the paired fixed missing-approval marker. Source now preserves that safe semantic marker; final real rerun is unverified. The log is `.logs/credentials-eks-kind.log`. No successful final transaction is claimed.
- `TestProtectedCAConfigOnDocker`: FAILED on the first implementation because an explicit configuration path made the read-only input directory the server home. Source now places server config under its writable CA home and specifies writable client MSP paths. A subsequent attempt was BLOCKED by Docker's missing engine pipe. These corrections still require the opt-in test; no successful Docker CA enrollment is claimed.
- The original local kind regression was preserved and extended with credential uniqueness/confidentiality assertions, but was not successfully rerun on final source because Docker was unavailable.
- PowerShell setup script parser and Python builder syntax were checked. No AWS resources, IAM changes, dependency installations or production enablement occurred.

Docker Desktop's backend log reports an inaccessible stale `C:/Users/dubes/AppData/Local/Docker/run/sailor-ingest.sock` during startup. Bounded start/restart attempts did not restore the Linux engine. The tool policy rejected removal of that single stale runtime socket; no alternate deletion method, factory reset or volume removal was attempted. Docker needs administrator/user recovery before final acceptance. The failed test requested cleanup of its own namespace; cleanup completion/current retained-network health cannot be reconfirmed while the engine is down.

After Docker is healthy, rebuild the protected CLI and final worker images, load the image into kind, run the disposable Docker CA test, then the two kind tests sequentially. Verify actual committed writes/queries, ten distinct credentials, clean Job/log output, controller Job adoption and cleanup. Inspect only test-owned resources; do not delete the retained user network.


## Verification after Docker recovery — 22 October 2026

This section supersedes the earlier Docker-blocked execution status; the failures above remain historical evidence. The rerun found and corrected the pinned `tlsCA` JSON-field casing in both the credential guard and registrar restriction. A unit regression covers the exact upstream field. Fabric CA also rewrites its client configuration after enrollment: the Docker client now copies the protected input into container tmpfs before execution, keeping the host delivery file read-only.

- The final scoped worker image was rebuilt and loaded into kind: `sha256:2d82e85d5cc43eb3d0a4d90eb445a26998211178cb5f5f0f8a4be73997c20e37` (local manifest-list digest).
- `go test ./... -count=1` and `go vet ./...`: passed. PostgreSQL integration remains opt-in and was not claimed by this run.
- `SMP_CREDENTIAL_DOCKER_E2E=1 go test ./internal/localfabric -run '^TestProtectedCAConfigOnDocker$' -count=1 -v -timeout 3m`: passed (2.14 seconds). The official Fabric CA performed registrar TLS enrollment, restricted peer registration and separate peer enrollment through protected configuration files. Log: `.logs/credentials-docker-ca-recheck.log`.
- `TestEKSWorkerLifecycleOnKind`: passed (175.21 seconds), real transaction `b5d2ea06f095fe5d15b4743a1beadd699af405e4003f0da04815e19629b46dcf`, both peers at height 9. Verified ten distinct credentials and absence from worker logs/Job manifest, scoped RBAC, channel brokering and Job adoption. This is kind evidence, not AWS/EKS evidence. Log: `.logs/credentials-kind-final.log`.

The required inline enrollment fields in pinned Bevel CRs, operator/Helm/audit access, certificate rotation and retained Secrets/PVCs remain security concerns. The legacy Docker profile also mounts a shared runtime tree into trusted local services; filesystem permissions do not provide strong isolation against privileged/root containers. Do not expose that development profile as a shared tenant service. These residual boundaries are not resolved by passing the credential-delivery tests.

- `TestLocalWorkerRegressionOnKind`: passed (170.53 seconds), transaction `b933e34e26a9a413f9a66c421d81d9901a7cc9d523947568d7f8a09fad3eeb71`, both peers at height 9. Ten distinct credentials and no worker-log/Job-manifest credential exposure verified. Both kind tests, including cleanup, passed in one sequential run (347.340 seconds). Final log: `.logs/credentials-kind-final.log`.
- Read-only follow-up confirmed the retained user network namespace `smp-n-968a212068be7d07` still had all eight deployments ready. Unrelated namespaces were left untouched. No AWS resources were contacted or created; production execution remains disabled.

Credential argument delivery and per-identity credential separation now have passing real local evidence. This does not eliminate the documented plaintext Bevel CR/Helm fields or qualify AWS/EKS, automated rotation, tenant isolation or secure data destruction.

## Enterprise follow-up

The pinned-controller compatibility investigation and compensating controls are recorded in `docs/security-remediation-matrix.md`. External certificate references are not enrollment-password Secret references; default Kubernetes-store validators still require passwords. No unsupported reference was substituted and no controller/dependency upgrade was installed. Full plaintext elimination remains BLOCKED pending a separately tested controller/pre-enrollment migration.

## Enterprise follow-up verification

The repeated enterprise baseline passed both real kind profiles. The final enterprise scoped test additionally verified exact cleanup inventory, actual UID-precondition rejection and unrelated-probe preservation while retaining real Fabric transaction and credential confidentiality assertions. Durable rotation plans and encrypted local backup primitives do not eliminate the residual inline CR fields or implement live rotation. See `enterprise-readiness.md` and `security-remediation-matrix.md` for exact status and limitations.
