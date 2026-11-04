# Sprint 1 secure local application integration

Status: backend local qualification PASS; UI-to-Fabric BLOCKED in the separate
console repository. Sprint 1 must not be described as fully complete. Phase F's
separate-repository exception is used for this backend-only delivery. All required
backend security, API end-to-end and regression gates passed before commit.
No production or AWS qualification is implied.
The starting commit is `1c7fca651f833020184b089544ad1b0359aafb7a`.

## Architecture and supported contract

The existing network draft, operation, deployment and PostgreSQL repositories are
used. The explicit `smp-secure-local-v1` target selects `SecureRunner`; legacy
runners reject this target before creating operations. The candidate controller
is pinned to the atomic qualification digest in
[`secure_profile.go`](../internal/deployment/secure_profile.go). The original
controller source, image selection and retained network are not changed.

Only Fabric 2.5.15, Fabric CA 1.5.15, LevelDB, Org1MSP/Org2MSP, one peer per
organization, three Raft orderers and `smpchannel` are supported. The complete
accepted design is [`secure-local-network.json`](../examples/secure-local-network.json).
Cloud targets, production environment, arbitrary placement, organization counts,
consensus and inline credentials fail validation. State database selection is
not exposed by this schema; unknown fields are rejected.

`fabric-api -mode local-development -deployment-mode secure-local` additionally
requires PostgreSQL, `-local-token-grants` and `-secure-local-config`. The trusted
runtime configuration binds an immutable kubeconfig hash, an explicit
`smp-sprint1-<eight hex digits>` disposable kind cluster, a loopback Kubernetes
endpoint, exact Docker provider identity, digest-pinned worker and runtime
images. It is server configuration, never request input. Changing a binding
prevents recovery until the original binding is restored.

## Authentication and authorization

Local token mode requires an explicitly supplied local-development environment.
Staging, production and omitted/ambiguous mode are rejected. The existing Okta
verification code is preserved; it remains a mandatory Sprint 2 production gate.
The fixed developer principal cannot start secure deployments.

The provisioning command generates 256 random bits and writes only the SHA256
digest with developer, tenant, project, operation permissions and expiry (at most
24 hours). It displays the client token once. Run it interactively without shell
transcription or output capture; do not put its output in evidence. Server grant
files need host ACLs appropriate to their owner; Unix mode bits alone do not
configure Windows ACLs. Neither tokens nor grant files belong in Git.

```powershell
go run ./cmd/fabric-local-token -mode local-development -developer YOUR_ID -tenant YOUR_TENANT -project YOUR_PROJECT -permissions read,write,deploy,retry,cancel -grants E:/Sathonate/.cache/local-grants.json -lifetime 1h
```

Every request except public authentication discovery requires a bearer token.
The middleware checks expiry and the exact read/write/deploy/retry/cancel/cleanup
permission. Client tenant/project/role headers cannot change server grants.
Tenant and project are encoded together into a stable SHA256-derived existing
PostgreSQL tenant key. Services and existing RLS repositories use that key;
different projects cannot read or retry one another's objects. `GET
/api/v1/projects` returns only the project in the current grant.

## Existing HTTP workflow

1. `POST /api/v1/validate`: validate the qualified design and return profile gates.
2. `POST /api/v1/networks`: persist the draft in the authenticated project scope.
3. `POST /api/v1/networks/{id}/deploy`: require current `If-Match` revision and
   `Idempotency-Key`; return the durable operation with HTTP 202.
4. `GET /api/v1/operations/{id}` and its events: persisted stages and safe errors.
5. `GET /api/v1/deployments/{operationId}`: actual readiness, CA/peer/orderer,
   chaincode and transaction observations in the existing health contract.
6. `POST /api/v1/operations/{id}/retry`: authorized, durable retry of failed work.
7. Existing health/log/cancel routes remain authenticated and scope checked.

Draft saving and planning do not imply deployment. A succeeded operation requires
actual chaincode commit/readback on both peers and process argument inspection.
The runner has bounded timeouts; cancellation stops its exact owned Job and
retains material. Customer cleanup is deliberately refused for this profile;
trusted fixture teardown removes the complete disposable cluster.

## Issuer trust and recovery

The API provisions separate namespace Roles and service accounts for the pinned
candidate and Go issuer. Neither receives cluster-admin, foreign Secret access,
cluster-wide Secret listing or RBAC escalation. The API's trusted local cluster
credential remains a development provisioning boundary, not a production broker.
Namespace adoption requires every scope/network/profile ownership label; no
request can supply a namespace or context.

The Go worker uses protected CLI stdin, private memory-backed workspace files
and Fabric CA APIs. It persists random enrollment credentials before registration,
then validated immutable identity Secrets before publishing external references.
CA registry credentials are mounted through the qualified Secret-only candidate
path. Public CRs and Helm values contain references, never private keys or usable
enrollment passwords. Separate signing/TLS registrars, channel administrators,
chaincode administrators and client identities are used. Direct channel
participation avoids cluster-scoped channel controllers.

Secure client transactions use the pinned official Fabric peer CLI with explicit
endorsement targets, CA roots and commit-event waiting. Readback and raw QSCC
ledger checks run against both peers with their client identities. The protected
SDK invocation failed after successful chaincode definition commit during an
earlier disposable attempt; its private diagnostics were deliberately suppressed,
so no unsupported root-cause claim is made. The secure path uses the official
peer interface already exercised by the controller qualification. Legacy runner
SDK behavior is preserved. The chaincode TLS mount uses the candidate's required
`tls.crt`, `tls.key`, `tlsroot.crt` mapping, separately from node material mapping.
QSCC uses strict hexadecimal transport decoding: Fabric 2.5.15's
[`--raw` implementation](https://github.com/hyperledger/fabric/blob/v2.5.15/internal/peer/chaincode/common.go#L155-L166)
prints a decimal byte slice, not binary protobuf. A regression test preserves
leading/trailing binary whitespace and rejects malformed CLI output.

PostgreSQL records prepared submission and running operation checkpoints. API
restart adopts the same worker Job; a failed Job is retried through the operation
API. Immutable materials are restored and completed CA/node steps reconciled.
Retries do not rotate existing enrollment credentials on API errors. This does
not qualify CA-root, signing-identity, auxiliary identity or Raft rotation.

## Reproduction and evidence

With Docker Desktop, Go, Python, kubectl, the pinned candidate and public Fabric
images available locally, and the dedicated `smp-postgres` fixture running:

```powershell
Set-Location E:/Sathonate/fabric-control-plane
$env:GOCACHE='E:/Sathonate/.cache/go-build'
$env:GOPATH='E:/Sathonate/.cache/go'
$env:GOPROXY='off'
$env:TMP='E:/Sathonate/.cache/resume-tmp'
$env:TEMP=$env:TMP
$env:GOOS='linux'; $env:GOARCH='amd64'; $env:CGO_ENABLED='0'
go build -trimpath -o bin/fabric-worker-secure ./cmd/fabric-worker
Remove-Item Env:GOOS,Env:GOARCH,Env:CGO_ENABLED
docker build -f tools/kubernetes/Dockerfile.secure -t sathonate/fabric-worker:secure-local .
python tools/sprint1/qualify.py
go test ./... -count=1
go vet ./...
```

`qualify.py` only creates fixtures, loads images and inspects/tears down the
disposable cluster. The real authenticated Go HTTP test invokes the application
manager, runner, issuer and Fabric workflow. It never calls the Python lifecycle
issuer. Database credentials and test tokens remain in process memory, not
arguments or evidence. `.logs/sprint1-qualification.json` and
`.logs/sprint1-api-kind.json` are runtime results, not committed source.

## Frontend boundary and production gates

The frontend is a separate repository at `E:/Sathonate/fabric-console`; its
existing files are untracked rather than a verified committed baseline. It uses
Okta/fixed local authentication and the older local target. UI-to-Fabric
acceptance is BLOCKED until that repository adopts local-token authentication,
authorized project discovery and this profile contract. Unit/build success does
not close that acceptance gate. No replacement demo UI is introduced here.

Before production: integrate Okta project membership and effective restricted
database roles; qualify a production issuer/provisioning broker and admission/CNI
isolation; address historical retained-controller key exposure through separately
authorized incident assessment and rotation; qualify broader certificate/CA/Raft
recovery, backups and secure destruction; complete actual AWS infrastructure,
IAM/storage/AZ qualification only after separate authorization. AWS and
production remain disabled throughout this sprint.

## Final executed matrix — 24 October 2026

| Gate | Result |
|---|---|
| Real authenticated API → Go runner/issuer → candidate → Fabric | PASS, 276.69s; 278.881s package total |
| Durable application restart / same worker Job adoption / failed-worker API retry | PASS |
| Channel, chaincode package/approval/commit, transaction readback and ledger convergence on both peers | PASS |
| Actual live peer certificates, CA and DNS trust verification | PASS, both peers |
| Public CRs, ConfigMaps, pod/deployment definitions, all Helm history, runtime logs | PASS; 12 Helm history records, 10 log streams, 84 actual private values inspected in memory |
| Actual worker child-process arguments | PASS; 206 samples, including 38 CA client and 19 protected CLI commands |
| Source/evidence comparison against actual private material | PASS, 240 files at runtime inspection |
| Cross-tenant/project API and persistence denial; viewer escalation denial; duplicate operation reuse | PASS |
| Controller/issuer foreign Secret and cluster privilege denial; foreign canary preservation | PASS |
| Go tests / vet / Docker CA prototype | PASS; 101 top-level cases; nine opt-in skips covered by the explicit integration runs |
| PostgreSQL integration / restricted-role RLS / recovery | PASS; 26 top-level cases; two kind skips executed below |
| Candidate controller security tests | PASS, 18 cases against unchanged candidate source |
| Atomic / lifecycle / evidence Python tests | PASS, 13 / 10 / 3 cases |
| Scoped-worker kind regression | PASS, 212.81s including cleanup; AWS explicitly unverified |
| Original local runner kind regression | PASS, 168.59s including cleanup |
| Existing console units / build | PASS, nine tests / build; no UI-to-Fabric claim |
| UI-to-Fabric acceptance | BLOCKED: console has no committed baseline and lacks local-token/project/secure-profile wiring |
| Cleanup / retained network | PASS: disposable cluster and regression namespaces removed; retained deployments 8/8 ready, read-only |
| Production / AWS | BLOCKED, disabled; no AWS resources created |

Final secure operation: `op_a82871e30b59be25ed9b73e6aa82f348` (retry of
`op_1d43628cb91b6403e2393bee9c4f7520`). Committed transaction:
`b4f6cd325c66100c4ac86a486f9265e3c6d0ab1cac7da769616d44696fbc680e`.
Tested worker runtime manifest:
`sathonate/fabric-worker@sha256:500a97af7e8dc99585306c6ce1893b66daf4b74e3a1a6490dff4bcfb31804fcf`.
Docker's source image index and the imported runtime manifest are recorded
separately; embedded worker and candidate binaries match the recorded build.

Evidence: `.logs/sprint1-build.json`, `sprint1-build-inputs.json`,
`sprint1-api-kind.json`, `sprint1-qualification.json`, `sprint1-final-cleanup.json`,
and aggregate `sprint1-evidence.json`. Failed preflight, issuer-name, image-policy,
chaincode mapping, SDK invocation and ledger framing attempts remain separate;
none substitutes for the clean final run. The earlier 9-stage/42-check and six
atomic-recovery results are the committed controller baseline, not newly rerun
standalone-harness results from this sprint.
