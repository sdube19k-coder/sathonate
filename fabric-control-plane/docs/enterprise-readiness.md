# Enterprise readiness — execution record

Scope: incremental local security and release preparation, with production execution disabled. The retained namespace `smp-n-968a212068be7d07` and unrelated resources must not be modified by tests. This report records limitations rather than equating local evidence with EKS qualification.

## Milestone 1: baseline

Historical pre-baseline observation (superseded by the reviewed commit below): `git status` showed the implementation as untracked and `git rev-parse --verify HEAD` fails. There is no source revision to attest. No files were reset or discarded. `tools/engineering/evidence.py` records an explicit null revision plus a deterministic hash of source inputs, binary hashes, actual local image IDs and checksums/results of allowlisted test logs. That content hash is reproducibility evidence, not a reviewed release commit or image signature.

Pinned Bevel: v1.14.0, commit `cd236ccc61d1d159b3dadb4625d08bb5682a8451`. Both worker profiles require the source-built namespace-scoped CLI with protected stdin transport and 0600 enrollment output. Old release CLIs fail the worker capability check. Existing AWS qualification requires an exact approved target/network digest and loopback administration; production mode and the cloud flag still reject execution.

Baseline `go test ./... -count=1`, `go vet ./...`, and the real protected Docker CA test passed. Dedicated existing PostgreSQL database tests passed for tenant/revision isolation, atomic submission, restart, idempotency, durable cancellation and scheduler fencing. Two kind cases correctly skipped during that PostgreSQL-only command; they run separately. Fresh kind baseline is in progress; logs/evidence distinguish skipped, failed and passed cases.

Artifacts: `.logs/enterprise-baseline-tests.log`, `enterprise-baseline-ca.log`, `enterprise-baseline-postgres.log`, `enterprise-baseline-kind.log`, `enterprise-baseline-evidence.json`. No credentials, log bodies or configuration files are embedded in the evidence manifest.

## Milestone 3: controller and destructive-operation boundary

Cloud cleanup now requires an administrator role plus write scope and a current resource-preview digest. The old EKS cleanup entry points fail closed. Preview binds the saved network and target digests, namespace UID and exact resource UIDs. Fabric resources must carry the matching namespace ownership label; Jobs, ConfigMaps and worker permissions must carry matching tenant/network labels. Older unlabelled resources are refused rather than silently adopted. New worker-created Fabric resources receive these labels.

Cleanup uses named API DELETE requests with UID preconditions and observes removal. It never submits namespace, Secret or PVC deletion. It revokes only the recorded worker access after workload deletion and preserves unrelated named resources. An interrupted cleanup requires a new inventory and explicit approval. Local development cleanup is preserved, but destructive HTTP routes now require administrator scope. Unit authorization, ownership, wrong-resource/path and replacement-UID tests passed; real conditional-delete enforcement is tested only on disposable ConfigMaps during the final kind run.

This does not contain a compromised cluster controller or prove CNI/admission enforcement. No controller RBAC or dependency was installed or broadened. The pinned operator still has cluster-sensitive responsibilities; shared-tenant release remains blocked on reviewed containment and live negative access tests.

## Milestone 4: lifecycle safety components

Implemented secret-free tenant/target/identity-bound maintenance plans, administrator-only planning/retrieval API, per-step approval and trusted-receipt state contract, simulated-receipt labelling, disabled live mutation adapter, PostgreSQL idempotency and revision checkpoints. Interrupted/failed/unverified/duplicate/cross-tenant cases pass unit tests. The dedicated PostgreSQL recovery/CAS integration passed (0.10 seconds). Exact procedure and residual live-adapter blocker are in `docs/credential-rotation-recovery.md`. No real identity was rotated or revoked.

## Milestone 5: local backup and retention controls

Implemented bounded AES-256-GCM bundle encryption/restoration with exact binding authentication, private no-overwrite outputs, redacted receipts, public-certificate-only inspection, metadata-only Secret/PVC retention inventory, exact cleanup preview/approval and UID-conditional deletion. Synthetic local roundtrip, tampering, wrong key/tenant, malformed nonce, private-key rejection and overwrite tests passed. See `docs/backup-retention-cleanup.md`. Live consistent ledger/CA/PostgreSQL backup/restore, cloud snapshots and secure destruction remain NOT VERIFIED; the tool does not silently export data or erase retained credentials.

## Milestones 6–7: readiness and gated acceptance preparation

Readiness now includes optional protected PostgreSQL TLS/role/schema checks, standard CSI availability and explicit unverified security/storage/image/database-isolation gates. Tests reject unsafe TLS configurations without connecting. Staged `fabric-acceptance` supports offline plan, opt-in preflight, approved deployment/duplicate check, observations, storage/RBAC probes, controlled single-node restart, fresh verification, Job UID adoption, cancellation/retry and previewed cleanup. Exact commands and unsupported/unverified acceptance gates are in `eks-qualification.md`. Harness unit tests use fake commands and verify no cloud access without opt-in/current approvals. No AWS execution or costs were authorized or performed. This is preparation, not completed AWS acceptance.


## Files changed for this request

- `internal/deployment/cleanup.go`, manager/EKS/Kubernetes runners and cleanup tests: exact bound inventory, metadata retention references, admin approval, UID deletes and removal observation; legacy broad cloud cleanup disabled.
- `internal/kubernetesfabric/worker.go`: namespace ownership labels on newly managed Fabric resources (including chaincode); existing unlabelled cloud resources are not automatically adopted for deletion.
- `internal/httpapi/{api,deployment,maintenance}.go`, `internal/identity/okta.go` and negative route tests: administrator destructive/maintenance boundaries and read-only rotation plan retrieval.
- `internal/maintenance/*`, `internal/persistence/postgres.go`, `cmd/fabric-maintenance`: durable secret-free lifecycle intent/checkpoints and authenticated encrypted local backup/certificate inspection primitives. Live credential mutation remains disabled.
- `internal/cloud/{readiness,postgres_readiness}.go`, tests and `cmd/fabric-eks`: opt-in secure PostgreSQL check plus explicit unverified enterprise gates.
- `internal/acceptance/*`, `cmd/fabric-acceptance`: staged opt-in EKS qualification harness with exact per-action approvals, fixed redacted output and no automatic cloud provisioning.
- `tools/engineering/evidence.py` and its tests: allowlisted local source/binary/image/test evidence, with no credentials or log bodies.
- Runbooks/matrix: `security-remediation-matrix.md`, `credential-rotation-recovery.md`, `backup-retention-cleanup.md`, `eks-qualification.md`; existing enrollment, EKS worker and verifier documents updated.

## Release gates that remain open

The implementation is not an enterprise release. The earlier missing-HEAD finding is superseded by reviewed initial commit `e378cbf66fa74f3c533231830ec9225ce3aaa353`; later milestone work is separately recorded below. Inline enrollment CR/Helm storage is not eliminated. No production rotation/revocation driver, consistent live ledger/CA backup/restore, secure destruction, effective controller/CNI/Okta isolation or real EKS/EBS/AZ qualification has passed. The acceptance harness automates individual staged checks, not every listed gate; missing evidence remains NOT VERIFIED. Operation/deployment forced RLS and explicit executor recovery authorization were added in the subsequent milestone below. The trusted executor still has cross-tenant authority and requires separately approved restricted database roles.

AWS prerequisites remain an owner-authorized account/region/budget, existing compatible cluster, explicit scoped role/access mapping, three AZ placements, EBS CSI IAM/KMS/storage, approved digest images/operator/dependencies, exclusive namespace policies and dedicated secure PostgreSQL configuration. No cloud resource was created, IAM changed, cluster dependency installed or cloud cost incurred during this task. Production execution remains disabled.


## Final verification so far

After Docker became unavailable during an initial final-build attempt, it was restored by the user. That failed build and refused PostgreSQL connections were infrastructure failures, not passing tests. The subsequent protected CLI build, Linux worker build, both worker Docker image builds and kind image load succeeded. Dedicated final PostgreSQL cases passed, including maintenance recovery/CAS (0.11 seconds), network tenant/revision isolation (0.16 seconds), atomic submission (0.11 seconds), deployment recovery/cancellation/fencing (3.14 seconds) and persisted restart state (0.05 seconds). Kind cases intentionally skipped in the database-only invocation and run independently.

Final Docker CA test passed (3.29 seconds): protected registrar enrollment, restricted registration and independent peer enrollment. Final full Go suite and `go vet ./...` passed. The two Python evidence tests passed. Focused acceptance authorization, encryption, rotation interruption/retirement denial, database TLS refusal, readiness/redaction, destructive-route authorization and exact cleanup tests passed; AWS behavior is mocked only in those unit cases.

Final scoped kind transaction: `948c4f1fb38615564733d6d1834de23396361b2161214250da9821fc9f1789e4`, peer height 9. It also verified ten unique credentials absent from Job/logs, positive/negative worker RBAC, channel broker/Job adoption and real UID-precondition cleanup denial with an unrelated ConfigMap preserved. This proves the conditional-delete transport on disposable objects, not cloud retention or hostile shared-controller containment.

Source content SHA256 at this final test run: `f3d4a94a15afd984a16de5edb5767d4ec3fac16dc326960245dfdbc83eeedcd8`. Git revision remains null/unavailable. Final scoped worker manifest-list digest: `sha256:b9219a3b2fdfa8590fe30b119e79dadb0e7997ff02f9f40edf516a20ef96e772`; Docker config/image ID: `sha256:ac38d4c171c8a34f073289da0705b8cefc7ad40a3002ffaeff5adc863e48375f`. Binary hashes and current source hash are in `.logs/enterprise-final-evidence.json`; that current manifest is authoritative if later documentation-only edits occur.

## Final release-gate check

The rebuilt API rejected `-mode production -deployment-mode eks-qualification` with exit 1 and rejected `-deployment-mode real` with exit 1. No configuration flag opened production execution. The final original-local regression committed transaction `c6b070ad0e938cb4492441d638bd73bf46b18a78842077d03cc9bd6002d8b5b1` on both peers at height 9 and verified ten distinct credentials absent from worker logs/Job metadata. Final cleanup completion is recorded in the test log, not inferred from transaction success.

Highest-priority next action: review and isolate the pinned-controller pre-enrolled certificate/Secret migration, including CA bootstrap storage and renewal, then qualify it on disposable kind. Do not enable AWS production merely because these local controls pass. Obtaining an authorized existing EKS environment is a separate owner action and does not waive this security blocker.

## Final local milestone outcome

Both final kind tests passed including their cleanup: scoped/EKS-shaped 167.84 seconds, original local 162.56 seconds; package total 331.932 seconds. Final log: `.logs/enterprise-final-kind.log`. Earlier baseline also passed (191.25 and 178.17 seconds); it used the previous image and is not substituted for the final run. The retained user's eight deployments were confirmed ready by a read-only check after final tests; no retained network, PVC or Secret was deleted.

Milestone summary: 1 baseline/evidence completed with missing-Git-HEAD caveat; 2 compatibility investigation/compensating controls completed but password-free migration BLOCKED; 3 local cleanup/admin/ownership controls implemented and tested, controller/CNI/enterprise isolation NOT VERIFIED; 4 durable lifecycle safety components implemented/tested, live rotation/renewal/revocation BLOCKED; 5 authenticated local bundle/preview controls implemented/tested, consistent network restore/secure destruction NOT VERIFIED; 6 readiness extensions implemented/tested with unproven checks explicit; 7 staged harness/runbook prepared and unit-tested, real EKS execution NOT RUN; 8 available final local regressions completed. Do not describe all eight milestones as fully enterprise-qualified.


## Current follow-up: reviewed Git, certificate prototype and RLS

Reviewed initial Git commit: `e378cbf66fa74f3c533231830ec9225ce3aaa353`. All 156 staged files were inspected, staged whitespace checks passed, and secret matches were reviewed as synthetic fixtures/runtime references. Credentials/private artifacts, binaries, logs/backups and generated images are excluded. Follow-up source changes are uncommitted; `.logs/milestone-evidence.json` records both this baseline revision and a current source-content hash with `sourceClean=false`. Earlier null-revision entries above are historical results, not the current provenance.

See `passwordfree-isolation-milestone.md` for the identity compatibility matrix, exact commands, protected issuance limitations, isolated upstream candidate and remaining isolation requirements. The real retained CR inspection reports `passwordFree=false`; no secret values were exported. The certificate Secret prototype and Docker CA reenrollment pass, but replacement controller reconciliation, restart, channel/chaincode/transactions and certificate replacement/failure recovery remain BLOCKED/NOT VERIFIED. Existing-runner tests must not be substituted for replacement-path evidence.

Operation/deployment tables now enable and force RLS. A real restricted database role verifies cross-tenant read/insert/update/delete denial and unauthorized recovery refusal. Scheduler recovery requires approved executor role membership (local superuser is development-only); no role/grant is automatically provisioned. PostgreSQL readiness requires all five protected tables. Application/SQL trust boundaries and effective production database logins remain separate qualification gates.

Live read-only controller authorization confirms cluster-wide Secret read and ClusterRoleBinding creation by the installed default service account. These privileges were not broadened or changed. Shared-tenant controller containment remains BLOCKED. New real kind negative requests validate scoped worker denial, not containment of the controller. No AWS resources, IAM changes, production enablement or retained credential rotations occurred.


### Follow-up final verification results

| Executed check | Exact result |
|---|---|
| `go test ./... -count=1 -v` with Docker CA opt-in | PASS: 91 top-level cases, 0 failures, 8 integration opt-in skips. The skipped database/kind cases were executed separately below; nested subtests are not included in this count. |
| `go vet ./...` | PASS, exit 0. |
| Dedicated PostgreSQL `go test ./internal/operations ./internal/persistence ./internal/deployment ./internal/maintenance -count=1 -v -timeout 4m` | PASS: 23 top-level cases, 0 failures; 2 kind-only skips, subsequently run below. Restricted-role RLS test 0.07s; recovery/retry/cancellation 2.07s. |
| Protected Docker CA prototype | PASS: dedicated invocation 2.34s; full-suite invocation also passed. Registration, separate enrollment, reenrollment and matching renewed cert/key trust verified. |
| Pinned original/candidate validator tests | PASS: 4 cases across original and isolated candidate peer/orderer packages. Proves validation behavior only. |
| Protected pinned CLI build, Linux worker build, worker Docker build and kind image load | PASS after correcting missing kind PATH with `E:/Sathonate/tools/kind.exe`; no controller dependency installed. |
| `TestEKSWorkerLifecycleOnKind` | PASS, 175.32s including cleanup. Real transaction `7ddf084cb8df0332a5c82714773571599a05a6d8ab42cda3f047655a5ffe7014`, height 9. |
| `TestLocalWorkerRegressionOnKind` | PASS, 168.28s including cleanup. Real transaction `914bfbc4915098dd518b796153df7b916221f7385337571ec77d73d3466afe76`, height 9. Package total 345.198s. |
| Python evidence regression | PASS: 2 tests, including private-config/log/inspection canary omission. |
| Retained user network read-only observation | PASS: eight deployments each 1/1 ready at 2026-10-22 11:09:01 UTC. No fresh transaction sent to the retained network. |
| Password-free replacement lifecycle / hostile controller/CNI isolation / real EKS | BLOCKED or NOT VERIFIED, not counted as passing existing-runner regressions. |

Final source-content SHA256: `618873f459e27ac027e4e895e2f39d0d2d361be2edcde419aea5a43c23dd1038`. Current baseline commit remains `e378cbf66fa74f3c533231830ec9225ce3aaa353`; follow-up changes are uncommitted. Source/binary/image hashes and allowlisted test results: `.logs/milestone-evidence.json`. Detailed current logs: `milestone-tests.log`, `milestone-postgres.log`, `milestone-kind.log`, `milestone-vet.log`, `milestone-cli-build.log`, `passwordfree-ca-prototype.log`, `passwordfree-pinned-validator.log`. Earlier failed attempts and corrections are documented in `passwordfree-isolation-milestone.md` and do not count as passing evidence.

Remaining priorities before AWS qualification: (1) complete and qualify the pinned controller/CA password-free delivery lifecycle, including TLS chaincode and certificate replacement/recovery; (2) remove shared-tenant controller Secret/RBAC escalation capability using a tested dedicated identity and channel broker boundary; (3) qualify effective admission/CNI isolation and separate production database roles; (4) qualify retention, backup/restore and cleanup, including the development-only namespace-wide cleanup limitation; (5) obtain an explicitly authorized existing EKS environment and perform actual IAM/EBS/AZ/Okta qualification. Production remains disabled and no cloud qualification is claimed.


## Current controller security milestone

Previous verified changes were committed after staged review as `4457e8e894f53fb7b5e0cad4e7cdee1aa8c6ce9e`. The isolated candidate lives entirely in `tools/controller-lab`; original runner/controller behavior and retained user resources are unchanged. See `controller-security-candidate.md` for its exact scope and reproduction.

Critical new finding: upstream orderer code logs private-key material, and a read-only sample of the retained operator logs contained private-key markers. The audit exports booleans only; no key or raw log body was exported. This is an unresolved credential-exposure failure. Do not confuse suppression in an isolated candidate with remediation of historical or retained-controller exposure. An owner-approved incident/rotation plan is required.

Before the protected-registry follow-up, the candidate was intentionally fail-closed and could not initialize CAs securely: the pinned registry password maps into Helm values and ConfigMaps. Auxiliary operations/admin TLS, live certificate replacement/recovery and cluster-channel authority still require a complete issuance/broker design. No successful full Fabric deployment through this candidate is claimed. It is not adopted. Existing-runner transactions and rejected password-free schema fixtures cannot satisfy that criterion.


### Controller follow-up regression results

| Gate | Result |
|---|---|
| Full Go suite, `go test ./... -count=1 -v` with Docker CA opt-in | PASS: 91 top-level tests, 0 failures, 8 explicit integration skips; database and kind opt-ins executed separately. |
| `go vet ./...` | PASS, exit 0. |
| Docker CA enrollment/reenrollment | PASS, 2.25s within full suite. |
| PostgreSQL integration | PASS: 23 top-level cases, 0 failures, 2 kind-only skips. Restricted-role isolation 0.11s; recovery/retry/cancellation 2.14s. |
| Scoped real kind Fabric | PASS: 216.13s including cleanup, both peers at height 9; transaction `e43c66cd8e1051df902874fae7f67cdc3d6be174dd7ffff0c90ad8063799a78d`. |
| Original local real kind Fabric | PASS: 199.16s including cleanup, both peers at height 9; transaction `11f6f4e541af1d9508bd61a61a1e86eeeaf394803e6d41e352e0e5729868543d`. Package total 416.983s. |
| Candidate source tests/build | PASS: 9 targeted Go tests across helper, peer, orderer, CA and TLS chaincode packages; Linux build and helper vet pass. Secret-read HTTP behavior is simulated in unit tests, not AWS evidence. |
| Lab safety / evidence redaction | PASS: 4 Python lab safety tests and 2 evidence canary/redaction tests. |
| Retained user network | PASS: read-only 8/8 deployment readiness after original-runner regressions; stored CR inspection still reports passwords present. No retained transaction, cleanup or rotation performed. |
| Retained controller credential exposure | FAIL: private-key markers observed in actual sampled logs. Raw logs not exported. |
| Password-free replacement Fabric lifecycle | BLOCKED: secure CA registry delivery, auxiliary identity lifecycle, channel broker authority and live renewal/recovery incomplete. |
| AWS qualification | BLOCKED / NOT RUN: no authorized AWS environment or cloud mutation. |

Current artifacts: `.logs/controller-final-evidence.json`, `controller-candidate-build.log`, `controller-candidate-kind.json`, `controller-regression-tests.log`, `controller-regression-vet.log`, `controller-regression-postgres.log`, `controller-regression-kind.log`, `controller-retained-health.json`, `controller-upstream-log-audit.json`, `controller-lab-safety-tests.log`, `controller-evidence-tests.log`. The final source revision and image/binary/source hashes are recorded by the manifest after committing the reviewed source; ignored evidence is not source-controlled and does not change that revision.

Before EKS qualification: resolve observed key-log exposure through owner-approved incident assessment and coordinated rotation; implement and qualify protected CA runtime registry/issuance; complete auxiliary/chaincode/channel certificate lifecycle and live recovery; prove controller/CNI/admission isolation; qualify restricted database roles and backups; then obtain explicit existing-EKS account/region/role/namespace/storage/image approvals and perform real IAM/EBS/AZ/Okta tests. A local kind result never qualifies AWS.


### Final disposable candidate result

PASS: all 14 real Kubernetes security probes, including own-namespace access, foreign Secret/Fabric denial, cluster Secret-list denial, RBAC escalation denial, foreign cleanup denial/preservation, actual stored rejected-CR password absence, safe refusal without Fabric workload creation, sampled candidate log key-marker absence, controller restart and post-restart CA refusal. The disposable cluster was removed. Probe completed 22 October 2026 at 13:56:28 UTC (19:26:28 IST). Tested candidate image: `sathonate/controller-candidate@sha256:58d73550049ad7ea6b8e0439c5bc2ab05d97e77248db3738c1d2eb19f5e6b05c`.

These were rejected schema fixtures, not an operational Fabric network. Full password-free initial deployment/channel/chaincode/transaction/reconciliation/renewal/recovery remains BLOCKED. The actual retained network still has enrollment passwords and private-key logging exposure; no claim of remediation is made. Final redacted evidence is generated against the final reviewed source commit; its `sourceRevision`, `sourceClean`, candidate input/binary matches and tested image digest are authoritative.

### Protected CA registry follow-up

See [controller-registry-followup.md](controller-registry-followup.md) for the next isolated candidate change, exact executed regression results and remaining security gates. The candidate passes only a Secret reference into Helm and builds credential-bearing CA configuration in memory at pod initialization. It remains separate from the working engine and retained controller. A CA-only result cannot qualify a replacement Fabric lifecycle or AWS readiness. Existing runner regressions passed again (217.96s scoped kind, 184.64s local kind); the retained network remains 8/8 ready. Historical private-key logging, complete issuance/recovery and effective enterprise isolation remain blockers.

Protected CA follow-up result: PASS, 22 real kind checks, root/TLS enrollment and reenrollment, actual public-resource/Helm-history/log password inspection, CA restart enrollment and disposable cleanup. Completed 22 October 2026 at 17:16:40 UTC. This closes CA registry delivery only; full replacement Fabric lifecycle and AWS qualification remain BLOCKED. See controller-registry-followup.md for tested image digest, commands, failed attempts and remaining exposure.

### Secure controller lifecycle follow-up

See [controller-lifecycle-qualification.md](controller-lifecycle-qualification.md) for the subsequent isolated node/auxiliary identity, channel, chaincode, Secret-only chart material and certificate lifecycle qualification. Its result matrix supersedes the older implementation-gap descriptions only where an actual passing test is recorded. The retained controller, its historical private-key exposure, the existing deployment engine and live credentials remain unchanged. The redacted final manifest records the reviewed source revision and tested image separately; a successful original-runner regression never qualifies the replacement controller or AWS.

Latest result, 23 October 2026: PASS, nine real disposable kind stages and 42 checks, including CA/node/auxiliary issuance, three Raft orderers, two peers, channel/chaincode, committed writes read on both peers, restart, missing-reference rejection/recovery, peer TLS renewal queued during controller outage, live renewed-certificate verification, actual public-resource/Helm/log/argv inspection and cleanup. Tested candidate: `sathonate/controller-candidate@sha256:a467cb2986f33d870477730b8ef3b0c56991280b14cf426091b2d28fa739b03a`. Fourteen candidate Go tests/build/vet, 91 root Go tests (eight integration skips executed separately), 23 PostgreSQL cases, Docker CA, protected CLI, ten Python lab tests and three evidence tests pass. Both original real kind regressions pass again: 183.58s scoped and 183.30s local. Retained readiness is 8/8 on a read-only check.

Milestone 2 is not fully complete for enterprise promotion: broader identity/CA/Raft rotation and partial-write failure recovery remain unqualified; production issuer/runner integration and effective CNI/admission isolation are pending. Original-controller private-key logging remains a confirmed FAIL outside the candidate. AWS qualification is BLOCKED, not run, and no production flag or default operator image is changed. The linked qualification report gives exact commands, technical blockers, safe alternatives and ordered EKS prerequisites; final commit/image/source hashes are in `.logs/controller-final-evidence.json`.

## Sprint 1: application-to-secure-controller integration checklist

Starting revision: `1c7fca651f833020184b089544ad1b0359aafb7a`, clean. Backend local qualification PASS; UI-to-Fabric remains BLOCKED in the separate console repository. The backend-only commit uses the explicit Phase F boundary and does not declare full Sprint 1 completion.

Discovery: `cmd/fabric-api/main.go` selects existing deployment modes; `internal/httpapi/deployment.go` owns start/status/cancel/health/log routes; `internal/deployment/manager.go` journals operations and results; `internal/operations` and `internal/persistence` implement PostgreSQL tenant scoping/RLS. `internal/deployment/kubernetes.go` is the original local runner (including a cluster-admin worker binding); `eks.go` provides scoped Jobs and recovery. Neither selects the secure atomic candidate. `internal/kubernetesfabric/worker.go` still generates operator enrollment fields and cluster channel CRs, so routing it to the candidate would fail or violate the public-material constraint. It must not be relabelled as secure. The secure standalone harness has independent registrar/identity issuance and direct channel participation, but is not an application runner. Candidate channel generation currently accepts exact Org1MSP/Org2MSP, orderer-1..3 and smpchannel only.

Authentication is fixed local-developer in local-development mode or Okta in production mode. There is no project entity, project membership claim, project repository or PostgreSQL project authorization policy. The console at `E:/Sathonate/fabric-console` is available; AuthGate uses Okta and the deployment screens use existing operation/status APIs. It has no authorized project selection or secure profile. UI acceptance remains unverified, not a replacement-screen task.

- [x] Confirm Git baseline, routes, workers, persistence, authentication and frontend.
- [x] Trace secure controller/material/issuer/channel implementation and identify reuse boundaries.
- [x] Introduce explicit versioned local secure profile with immutable provenance, strict qualified topology and a fail-closed legacy-runner boundary.
- [x] Establish explicit local-only bearer grants with server-bound developer/tenant/project/permissions and expiry; preserve Okta for Sprint 2.
- [x] Add project isolation at service and PostgreSQL layers before accepting customer operations.
- [x] Implement application-owned protected Go issuance and immutable material checkpoints; do not invoke or wholesale copy the Python harness.
- [x] Add namespace-exclusive candidate/issuer provisioning with explicit pinned controller and worker images; no original-controller fallback or cluster-admin worker.
- [x] Integrate direct Fabric channel/chaincode/transaction operations with durable stages and idempotent restart/retry.
- [ ] Wire existing console to authenticated project/secure-profile APIs.
- [x] Qualify actual authenticated API-to-Fabric path, worker restart, retry, runtime inspections and retained read-only health on fresh disposable kind.
- [x] Run Go/vet/PostgreSQL/local/scoped-worker/candidate/Python/UI regressions; publish matching redacted evidence.
- [x] Review and commit only after all mandatory security and API end-to-end gates pass.

Fail-closed policy: the existing local/EKS worker and the Python lifecycle harness must never be treated as an integrated secure runner. Production/AWS stay disabled; the original controller and retained credentials remain unchanged. Atomic peer TLS recovery does not establish broader Fabric identity rotation or a production issuer.

Implementation and reproduction: [Sprint 1 secure integration](sprint1-secure-integration.md). The secure profile uses the existing Go HTTP API, PostgreSQL and operation executor. UI acceptance is separately BLOCKED at the other repository; frontend unit/build results do not imply UI-to-Fabric success. No commit is allowed while a mandatory acceptance gate remains incomplete.

Final backend qualification: PASS, authenticated API deployment with durable restart/adoption/retry, committed transaction read on both peers, strict ledger decoding and actual TLS/credential/RBAC inspections. Go 101 cases plus vet, PostgreSQL 26 cases, candidate 18 cases, Python 13/10/3 cases and both kind regressions pass. The retained network remains 8/8 ready, read-only, and all temporary resources are removed. Console units/build pass but its real UI acceptance remains BLOCKED. See the linked report for exact evidence, failed attempts, source/build provenance, commands and production gates.
