# Password-free enrollment and isolation investigation — 22 October 2026

## Reviewed baseline

Initial commit: `e378cbf66fa74f3c533231830ec9225ce3aaa353` on `codex/control-plane-foundation`. The 156 staged files were reviewed before committing. The staged secret scan matched synthetic tests and runtime variable references; no actual credential was identified. Private configuration, kubeconfigs, credentials, binary outputs, generated images, logs, archives and backups are ignored. Subsequent milestone changes are separately visible in the working tree; the initial commit alone does not attest those changes. Evidence includes a deterministic source-content hash.

## Smallest safe prototype and actual compatibility

`internal/certificateprototype` builds an immutable, namespace-bound Kubernetes Secret from a verified pre-enrolled ECDSA certificate/key and trusted CA. Its external reference uses only fields present in pinned Bevel `cd236ccc61d1d159b3dadb4625d08bb5682a8451`. The Secret necessarily contains a private key: it is intended only for protected Kubernetes delivery, not ordinary JSON, logs, evidence or source control. This prototype is not connected to the runner and does not apply resources. Its in-memory objects are not persisted locally. Default JSON/Go formatting of the object excludes the private Secret; tests verify this redaction boundary. Only explicit protected Secret delivery may access the Secret payload.

The opt-in Docker CA test registers a separate identity with a separate credential, enrolls and reenrolls it, selects the private key matching the renewed certificate, and validates the external Secret prototype. Protected client/server configuration files remain necessary during issuance. They reside in test temporary storage; Windows ACL enforcement by Go temporary files is not a production secret-store guarantee. Container diagnostic output and the private bundle are captured privately and never published, even on failure. Container removal and temporary-directory removal are best effort, not secure media erasure.

`tools/prototype/check-pinned-external.py` archives the exact pinned revision separately. Actual upstream peer and orderer validator tests demonstrate rejection of external certificate references without enrollment passwords. A validation-only candidate patch in that archive accepts complete same-namespace signing/TLS references and rejects foreign or incomplete references. It is not installed, is not a complete reconciliation patch, and must not be shipped as a controller fix.

| Identity / lifecycle | Determination |
|---|---|
| Peer signing and TLS | External certificate Secret read paths exist; default Kubernetes-store validator still requires enrollment password. Validation-only patch tested, full reconciliation NOT VERIFIED. |
| Orderer signing and TLS | Same validator blocker; admin TLS and restart/consenter behavior require separate verification. |
| CA bootstrap | Registry identities contain inline `pass`; certificate/key Secret support does not replace bootstrap registry password delivery. No password-free replacement verified. |
| Channel admin | Existing wallet Secret delivery supports using enrolled cert/key without a password in channel resources; upstream identity issuance still needs protected credentials. No replacement lifecycle qualified. |
| Chaincode | Existing sample uses the supported plaintext in-cluster development chaincode service. Upstream TLS-enabled deployment controller reads `spec.credentials.enrollsecret`; password-free TLS chaincode path NOT VERIFIED. |
| Renewal | Official CA reenrollment and renewed certificate/key validation PASS in disposable Docker. External Secret replacement/controller reconciliation/rollback NOT VERIFIED. Immutable certificate Secrets require versioned names and explicit controlled reference changes. |
| Initial deploy/restart/reconcile/channel/committed transaction on replacement | BLOCKED until a complete controller/CA delivery design is implemented. Existing runner regressions do not qualify this prototype. |

Read-only inspection of the retained network's actual CRs exports only credential field paths, resource kind/name and timestamps in `.logs/milestone-stored-cr-inspection.json`. Populated CA registry passwords and peer/orderer enrollment secrets remain. `passwordFree` is **false**. No CR was scrubbed or credential rotated. Existing Helm release history, etcd, audit payloads, backups and privileged controller memory may retain credentials. Do not describe the exposure as resolved.

## Database isolation

Operations and deployments now have enabled, forced RLS, covering SELECT/INSERT/UPDATE/DELETE. Tenant-scoped restricted roles see only the transaction-local `sathonate.tenant_id`. A real disposable NOLOGIN/NOBYPASSRLS role in the dedicated test database verifies own-tenant writes and cross-tenant read/update/delete/insert denial. Network/revision and maintenance protections remain in place. Readiness now requires forced RLS metadata on all five tables.

The scheduler deliberately recovers all tenants and therefore requires explicitly approved membership in a dedicated `sathonate_executor` NOLOGIN group; startup fails closed without that capability. No role or grant is created automatically. Local superuser behavior remains supported for the existing developer environment; it is not a production security boundary. An administrator must provision a NOSUPERUSER/NOBYPASSRLS executor login, explicitly approve group membership and narrowly grant table access, separate tenant roles and migration ownership, lock the schema/search path, enforce verified TLS, and test those actual logins before enterprise use. Never grant this group to tenant pools. The current shared trusted API/executor process is not split into separately authenticated database services. RLS with an application-controlled tenant GUC does not protect against an attacker allowed arbitrary SQL or compromise of the trusted executor. A transaction-local tenant value must never be accepted directly from request data.

## Controller and cleanup boundaries

Pinned controller RBAC includes cluster-wide Secret access and cluster RBAC mutation. Read-only live checks confirm the installed `smp-system:default` service account can get Secrets across namespaces and create ClusterRoleBindings; namespace deletion is denied. This is a **shared-tenant security blocker**. No live controller privileges were changed because the retained network depends on it. Safe minimization requires separating namespaced reconciliation from cluster channel/broker operations and testing a dedicated controller identity in a disposable environment; an untested Role replacement could break reconciliation without establishing containment.

The scoped worker test now performs actual cross-namespace Secret and cluster-channel GET attempts and requires Kubernetes Forbidden responses, in addition to permission discovery. These use its disposable namespace, not customer identities. Existing cross-tenant HTTP, network/operation, approved target/channel and cleanup tests remain. Cleanup remains admin-approved, exact ownership/UID-bound and fail-closed; it does not delete customer namespaces, Secrets or PVCs. The local-only runner still performs whole owned-namespace deletion plus its worker ClusterRoleBinding cleanup; it is suitable only for the dedicated development cluster, not shared-tenant production cleanup. Its namespace ownership check does not provide cloud-style per-resource UID approval. The regression fixtures invoke it only for their own freshly created namespaces/channels. Compromised-controller containment, effective CNI/admission isolation, hostile tenant scheduling and secure deletion of retained Helm/Secrets/PVC data remain NOT VERIFIED. No live cleanup or rotation policy changed.

## Reproduction commands (PowerShell, repository root)

```powershell
$ErrorActionPreference='Stop'
$PSNativeCommandUseErrorActionPreference=$true
$env:GOCACHE='E:\Sathonate\.cache\go-build'
$env:GOPATH='E:\Sathonate\.cache\go'
$env:GOPROXY='off'
go test ./... -count=1
go vet ./...
$env:SMP_CREDENTIAL_DOCKER_E2E='1'
go test ./internal/localfabric -run '^TestProtectedCAConfigOnDocker$' -count=1 -v -timeout 3m
Remove-Item Env:SMP_CREDENTIAL_DOCKER_E2E
python tools/prototype/check-pinned-external.py --source E:/Sathonate/dependencies/bevel-operator-fabric --build-dir E:/Sathonate/.cache/passwordfree-validator
python tools/kubernetes/build-scoped-cli.py --source E:/Sathonate/dependencies/bevel-operator-fabric --build-dir E:/Sathonate/.cache/bevel-scoped-cli --output bin/kubectl-hlf-scoped
$env:GOOS='linux'; $env:GOARCH='amd64'; $env:CGO_ENABLED='0'
go build -trimpath -o bin/fabric-worker ./cmd/fabric-worker
Remove-Item Env:GOOS,Env:GOARCH,Env:CGO_ENABLED
docker build -f tools/kubernetes/Dockerfile --target worker -t sathonate/fabric-worker:local -t sathonate/fabric-worker:eks-scoped .
& E:/Sathonate/tools/kind.exe load docker-image sathonate/fabric-worker:local sathonate/fabric-worker:eks-scoped --name smp-qualification
$env:SMP_EKS_LOCAL_E2E='1'
$env:SMP_TEST_KUBECONFIG='E:\Sathonate\.cache\smp-qualification.kubeconfig'
go test ./internal/deployment -run '^(TestEKSWorkerLifecycleOnKind|TestLocalWorkerRegressionOnKind)$' -count=1 -v -timeout 20m
Remove-Item Env:SMP_EKS_LOCAL_E2E,Env:SMP_TEST_KUBECONFIG
python -m unittest discover -s tools/engineering -p 'test_*.py' -v
python tools/engineering/evidence.py --output .logs/milestone-evidence.json
```

PostgreSQL integration additionally requires `SATHONATE_TEST_DATABASE_URL` injected from protected local configuration into the dedicated `fabric_smp_tests` database, never printed or placed in command arguments. Executed: `go test ./internal/operations ./internal/persistence ./internal/deployment ./internal/maintenance -count=1 -v -timeout 4m`. The opt-in kind tests intentionally SKIP in that database-only invocation and execute separately. The test administrator needs permission to create/drop its own disposable NOLOGIN role. Never run these schema tests against a customer database.

The upstream validator build initially lacked cached dependencies with GOPROXY off. Retrying with `https://proxy.golang.org,direct` populated only the local Go module cache; no cluster dependency was installed. A nil orderer fixture was corrected before both validator suites passed. The first CA prototype failed due to old/new keys in the reenrollment keystore; matching the renewed key corrected it. The first image-load invocation lacked kind on PATH; PowerShell reported a command error despite the wrapper retaining the prior native exit code. The documented absolute executable corrected it and reproduction commands now use fail-fast PowerShell settings. None of those failed attempts count as passing evidence.

Final results are recorded in `enterprise-readiness.md` and the redacted manifest. No AWS calls/resources, paid infrastructure, IAM changes, production enablement, retained-network mutations or silent live rotation were performed.


## Source changes in this follow-up

- `internal/certificateprototype/*`: verified ECDSA material, immutable same-namespace Secret/reference prototype, redacted default serialization and negative material tests.
- `internal/localfabric/credentials_docker_test.go`: real renewal, matching renewed key selection and privately inspected certificate bundle; fixed redacted failure diagnostics.
- `tools/prototype/*`: pinned original/candidate validator tests in a separate archive; actual stored-CR field-path-only inspection.
- `internal/operations/postgres.go`, `isolation_test.go`: forced RLS, explicit recovery authorization and real restricted-role negative tests.
- `internal/cloud/postgres_readiness.go`: all five protected tables required.
- `internal/deployment/eks_local_test.go`: effective forbidden Secret/channel reads in real scoped kind execution.
- `tools/engineering/*`: current run evidence allowlist and stored-inspection digest; canary tests reject secret propagation into evidence.
- Engineering report, security matrix and this runbook: current provenance, compatibility, commands and blockers.
