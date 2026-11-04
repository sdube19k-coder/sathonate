# Atomic peer material candidate

This experiment continues baseline `5aa115c9b41a4384ef73a3ddf6fbfeb421e2ffe3` and Bevel pin `cd236ccc61d1d159b3dadb4625d08bb5682a8451`. It is not selected by an application runner. The original controller, retained network and passing `controller-lab` candidate are unchanged. AWS qualification and production execution remain disabled.

## Material flow and recovery design

The passing baseline obtains each external signing/TLS certificate through a namespace-authorized Secret reference, obtains auxiliary material from owned Secrets, validates it, writes conventional chart Secrets, then calls Helm and rolls out public checksum changes. Those separate mutable Secret writes cannot form a Kubernetes transaction. A crash can leave mixed generations mounted by a restarted workload.

`tools/controller-atomic/build.py` creates a separate patched controller and chart. The peer path validates signing certificate/key/root, TLS certificate/key/root and operations certificate/key together, before writing any bundle. Its bundle name hashes public certificates, namespace, FabricPeer name and UID; private key bytes are excluded from the name. Each bundle is an immutable Secret with exact owner UID and namespace labels. Existing bundles must match all data and ownership; no update or deletion is attempted. Missing or invalid inputs fail closed. Intermediate-root configurations and trust-root changes are rejected; they require separate coordinated channel/MSP procedures.

All eight peer certificate volume projections refer to the same validated immutable Secret. The pod-template reference changes as one Deployment update. Public CR source references express desired state; they do not assert that a rollout has completed. A namespace-owned public checkpoint ConfigMap records attempt, phase, desired, active and previous bundle names. A checkpoint does not authorize a switch: material is revalidated on retry. Public Helm values retain public certificates and bundle references, never private keys. Existing issuer input Secrets remain protected inputs; this experiment does not remove the need to store private keys in Kubernetes Secrets.

The peer Helm history limit is five, preserving rollback material. A pending upgrade rolls back its public release and immutable reference before retrying the validated desired state. A deployment is acknowledged only after its generation is observed, updated/ready/available replica counts match, required volume projections match, and Helm reports the same deployed chart configuration. A crash after rollout can acknowledge that existing release without another upgrade. Previous immutable Secrets are retained for recovery; automatic garbage collection is not implemented.

This is atomic material selection per pod, not an atomic transaction across Secrets, CRs, Helm history, Deployments and Fabric channel state. The upstream peer chart uses Recreate; a valid rollout may interrupt availability. Required evidence is deterministic recovery and successful subsequent transactions, not zero downtime. Only peer TLS replacement is exercised by this qualification; it does not qualify signing, operations, administrator, orderer/Raft, CA/root or chaincode identity rotation.

## Reproduction commands

Run from `E:/Sathonate/fabric-control-plane/` in PowerShell with the existing Go/Python caches and Docker Desktop Linux engine. The established local prerequisites remain in `controller-lifecycle-qualification.md`. Run full-network tests sequentially to avoid competing with the retained network for Docker memory.

```powershell
$env:GOCACHE='E:/Sathonate/.cache/go-build'
$env:GOPATH='E:/Sathonate/.cache/go'
$env:GOPROXY='off'
python -m unittest discover -s tools/controller-atomic -p 'test_*.py' -v
python tools/controller-atomic/build.py --source E:/Sathonate/dependencies/bevel-operator-fabric --build-dir E:/Sathonate/.cache/controller-atomic-candidate --output bin/controller-atomic
docker build -f tools/controller-lab/Dockerfile -t sathonate/controller-candidate:atomic E:/Sathonate/.cache/controller-atomic-candidate
python tools/controller-atomic/qualify.py --kind E:/Sathonate/tools/kind.exe --cache E:/Sathonate/.cache --source E:/Sathonate/.cache/controller-atomic-candidate --output .logs/controller-atomic-kind.json
python tools/controller-atomic/evidence.py --output .logs/controller-atomic-evidence.json
```

The harness creates a uniquely named kind cluster, verifies its provider identity, uses only its generated kubeconfig/context, and deletes only that cluster. Admission-denial fixtures are cluster-scoped policies installed only there, restricted to the test namespace and exact resource/bundle. No dependencies or policies are installed in the retained cluster. `SMP_ATOMIC_LAB_FAULTS=1` is set only on this disposable candidate: its fixed ConfigMap gates can hold reconciliation or terminate the process with code 73. This diagnostic mechanism must remain absent from any adopted production profile.

The inherited full Fabric lifecycle runs before six failure cases. Each case must verify the previous or newly rolled-out live certificate and committed/query results on both peers, followed by deterministic recovery, immutable Secret reuse, retained rollback material and credential inspection. Server-side dry runs wait for admission denial to become effective before changing desired state. Process-crash tests verify exit code 73, and inspect previous-container logs in memory. The final inspection covers actual CRs, public resources, decoded Helm histories, current logs and sampled process arguments. Evidence exports allowlist statuses, booleans, public certificate hashes, image references and source hashes; raw resources/logs/private material are not exported.

Repeated deliberate crashes reach the kubelet's observed five-minute backoff. The harness allows 660 seconds to observe two requested crash/restart cycles and 420 seconds for subsequent recovery. These are bounded test budgets; no kubelet backoff or workload settings are changed. An earlier 150-second crash-observation budget was insufficient for this legitimate backoff and is recorded as a failed qualification attempt.

Before each transaction, the harness requires two consecutive actual TLS handshakes to both peers and all three orderers, with CA and hostname verification. This handles service/listener propagation after rollout; Kubernetes Deployment readiness alone can precede the Fabric port accepting connections. An untrusted certificate fails immediately. Writes are not retried by this readiness gate.

## Qualification status

Runtime qualification PASS on the final separate atomic image: all six failure cases, the nine-stage/42-check baseline, final runtime credential inspection and disposable cleanup completed. This qualifies the isolated peer-TLS recovery experiment only; production adoption and the rest of Milestone 2 remain blocked. See the final result below; earlier failed attempts remain historical evidence.

Verified regression commands and results for this change:

| Command / scope | Observed result |
| --- | --- |
| `go test ./... -count=1 -v`, `SMP_CREDENTIAL_DOCKER_E2E=1` | PASS: 91 tests; 8 integration skips, covered separately below. |
| `go vet ./...` | PASS, exit 0. |
| `go test -p 1 ./internal/operations ./internal/persistence ./internal/deployment ./internal/maintenance -count=1 -v -timeout 4m`, dedicated test PostgreSQL URL injected privately | PASS: 23 tests; 2 kind-only skips. Uses only `fabric_smp_tests`. |
| `go test ./internal/localfabric -run '^TestProtectedCAConfigOnDocker$' -count=1 -v`, Docker opt-in enabled | PASS: actual protected CA enrollment/reenrollment. |
| `go test ./internal/deployment -run '^(TestEKSWorkerLifecycleOnKind\|TestLocalWorkerRegressionOnKind)$' -count=1 -v -timeout 20m`, `SMP_EKS_LOCAL_E2E=1`, explicit qualification kubeconfig | PASS: scoped-worker 188.70s; local-worker 193.29s. Both committed/query results on both peers, with cleanup. Local kind evidence only. |
| Atomic build command above | PASS: 18 targeted upstream Go tests, candidate/controller/main vet and Linux build. |
| `python -m unittest discover -s tools/controller-atomic -p 'test_*.py' -v` | PASS: 13 tests, no skips. Includes evidence/error redaction, trusted transport gating and refusal of incomplete qualification. |
| `python -m unittest discover -s tools/controller-lab -p 'test_*.py' -v` | PASS: 10 tests. |
| `python -m unittest discover -s tools/engineering -p 'test_*.py' -v` | PASS: 3 redaction/evidence tests. |
| Protected pinned CLI build below | PASS, exit 0. |

```powershell
$env:SMP_EKS_LOCAL_E2E='1'
$env:SMP_TEST_KUBECONFIG='E:/Sathonate/.cache/smp-qualification.kubeconfig'
go test ./internal/deployment -run '^(TestEKSWorkerLifecycleOnKind|TestLocalWorkerRegressionOnKind)$' -count=1 -v -timeout 20m
Remove-Item Env:SMP_EKS_LOCAL_E2E,Env:SMP_TEST_KUBECONFIG
python tools/kubernetes/build-scoped-cli.py --source E:/Sathonate/dependencies/bevel-operator-fabric --build-dir E:/Sathonate/.cache/bevel-cli-scoped --output bin/kubectl-hlf-scoped
```

Recorded unsuccessful qualification attempts are retained under ignored `.logs/`, without raw sensitive diagnostics:

- `controller-atomic-exploratory-failure.json`: post-rotation peer-restart transaction failed on the exploratory image. Subsequent chart acknowledgement changes prevent redundant upgrades while unchanged workloads become ready; the later nine-stage/42-check run passed this transaction.
- `controller-atomic-api-timeout.json`: disposable Kubernetes API GET timed out during orderer initialization. Its disposable cluster was removed. Both independent real kind regressions then passed.
- `controller-atomic-backoff-budget-failure.json`: all nine stages and 42 baseline checks passed, and the first five fault cases passed. The final case exceeded the harness's old 150-second observation budget while kubelet reported `back-off 5m0s`, exit 73. The test budget correction is described above. This attempt is FAIL, not qualification success.
- `controller-atomic-transport-readiness-failure.json`: a post-rotation restart transaction encountered `connection refused` after Deployment rollout reported ready. The trusted TLS transport gate described above was added before subsequent qualification. This run failed and was cleaned up.

The current final run writes `controller-atomic-kind.json`; `controller-atomic-evidence.json` exports its redacted results, source revision/content hash, build-input/binary match checks, tested image and test-log hashes/counts. Empty vet/build output is not itself treated as a passed test; the exit results above were observed when executing the commands. The source revision is captured again after committing the reviewed changes.

## Remaining security gates

1. Integrate the separately qualified candidate into an explicitly approved production issuer/runner and migration design; this experiment is not adopted. Per-pod atomic selection is not a transaction across Kubernetes/Helm/Fabric state.
2. The retained original controller has a previously confirmed private-key logging exposure. This experiment neither changes that controller nor remediates historical logs/keys. Owner-approved incident assessment and coordinated migration/rotation remain required.
3. The candidate Role reads Secrets throughout its own exclusive namespace. Independent tenants must not share it. Effective CNI/admission enforcement and a production issuer/broker require separate qualification.
4. Old immutable bundles consume quota and retain historical private keys. A separately authorized, ownership-bounded garbage-collection/backup/retention policy must preserve active and rollback references. Whole disposable-cluster teardown is not production per-resource cleanup qualification.
5. Expired-certificate recovery, root/issuer rotation, Raft consenter updates, revocation, CA restore and non-peer-TLS identities remain unqualified. Never silently rotate retained credentials.
6. AWS prerequisites remain those in the baseline report: an explicitly authorized existing EKS target/role, exclusive namespaces, approved image digests, encrypted Secrets/EBS/backups, storage/AZ policies, restricted PostgreSQL and actual EKS isolation, deployment and cleanup tests. No AWS resources are assumed or created here.

## Historical resumed review before final successful run: 23 October 2026

The latest completed runtime attempt is **FAIL**, not qualification success. Its `.logs/controller-atomic-kind.json` records all nine baseline stages and 42 checks passing, plus five of six atomic cases passing. `before-ack` verified the new live certificate and committed/query transactions on both peers before acknowledgement, but recovery was not verified: a subsequent Kubernetes pod GET timed out. The attempt records disposable-cluster removal. The other five cases record previous-network operation, deterministic recovery, immutable bundle reuse, retained previous material and committed/query results on both peers.

| Failure boundary | Runtime result | Availability / recovery evidence |
| --- | --- | --- |
| Before Secret creation | PASS | Previous material operational; recovered after admission denial removed. |
| After Secret creation, before checkpoint reference publication | PASS | Previous material operational; exit-73 recovery reused immutable bundle. The source CR reference is desired input and already changed; this boundary is the public checkpoint/chart output reference. |
| During Helm pending upgrade | PASS | Previous material operational; pending release recovered after exit 73. |
| Before workload rollout | PASS | Previous material operational; recovered after Deployment admission denial removed. |
| Controller restart at prepared checkpoint | PASS | Previous material operational; two exit-73 cycles recovered. |
| After rollout, before completion acknowledgement | FAIL / recovery NOT VERIFIED | New certificate and transactions passed before acknowledgement; subsequent observation timed out. |

A fresh runtime attempt is **BLOCKED** in the resumed session: Docker Desktop Linux-engine named-pipe access is denied by the execution sandbox, including after network permission was granted. No disposable or retained cluster was modified in this session. Retained readiness was not freshly observed; the earlier 8/8 result remains historical. No new commit is justified until the final runtime qualification is complete. Existing staged and unstaged implementation changes are preserved.

The exact atomicity limit remains per-pod material selection by one Deployment template update. Kubernetes Secrets, CR updates, Helm release storage and checkpoint acknowledgement are separate operations. Immutable bundles and revalidation support forward recovery; Helm rollback is limited to an available successful release and cannot reverse Fabric channel/MSP state. Recreate rollouts may interrupt service. Signing/operations/root/CA/orderer/Raft rotation remain outside the peer-TLS evidence.

Priority blockers: (1) obtain execution access to Docker and complete all six cases plus final credential inspection and cleanup on the final image; (2) review and commit that verified source and regenerate matching provenance; (3) address original-controller historical private-key exposure through an authorized incident/migration procedure; (4) qualify broader identity/trust/Raft recovery, bundle retention/garbage collection and effective tenant isolation. Production and AWS remain disabled.

Resumed checks: separate atomic build PASS (18 targeted upstream Go tests, vet and Linux compilation); root `go test ./... -count=1` PASS and `go vet ./...` PASS; atomic Python tests 13/13 PASS; baseline Python tests 10/10 PASS; engineering evidence tests 3/3 PASS; `git diff --check HEAD` PASS. Initial root/Python invocations hit sandbox temporary-directory permission failures; reruns with TMP/TEMP set to `E:/Sathonate/.cache/resume-tmp` passed. Docker/PostgreSQL integration opt-ins and full runtime qualification were not rerun. These checks do not supersede the failed runtime result.


## Final verified atomic recovery result

Completed 23 October 2026, 22:13:41 IST (`2026-10-23T16:43:41.988851+00:00`), exit 0. Start `2026-10-23T16:11:54.264283+00:00`; elapsed 1907.72 seconds including setup and cleanup. Tested image: `sathonate/controller-candidate@sha256:427b352b20576258efdb59c9fc266ec01fe7b7f1419ce1e5d7ead488b10cba6c`. The original controller, retained network and baseline candidate were not changed. No retained transaction or rotation was performed; retained 8/8 readiness was not freshly checked in this session.

| Boundary | Result | Operational and recovery evidence |
| --- | --- | --- |
| Before Secret creation | PASS | Previous certificate and both-peer transactions passed during denial; new certificate and transactions passed after recovery. |
| After Secret creation / before checkpoint publication | PASS | Previous network operational through exit 73; immutable bundle reused after recovery. Source CR is desired input, not completion acknowledgement. |
| During Helm pending upgrade | PASS | Previous network operational; release recovered and desired certificate served with successful transactions. |
| Before workload rollout | PASS | Previous network operational during admission denial; desired rollout recovered after denial removal. |
| Controller restart at prepared checkpoint | PASS | Two exit-73 cycles; previous network operational; deterministic recovery and transactions. |
| After rollout / before acknowledgement | PASS | New network operational before acknowledgement; recovered controller, expected live certificate, committed/query transactions on both peers and ready acknowledgement verified. |
| Full candidate baseline | PASS | All nine stages and 42 checks. |
| Final credential inspection | PASS | Actual CRs, ConfigMaps/public resources, decoded Helm values/manifests/history, sampled process arguments, current workload logs and previous crashed-controller logs inspected without exporting private data. |
| Namespace/ownership/foreign protections | PASS | Inherited negative authorization checks and immutable owner validation remain in force. |
| Disposable cleanup | PASS | Only `smp-qualification` remained in `kind get clusters`; generated kubeconfig removed. |
| Production/AWS/broader Fabric identity rotation | BLOCKED / NOT QUALIFIED | No adoption, cloud mutation or broader rotation claim. |

An independent read-only Kubernetes observer watched the final held checkpoint and subsequent ready acknowledgement in the same disposable context. It confirmed controller availability, identical active material reference, Helm revision **13 before and after**, identical peer pod UIDs and unchanged per-container restart counts. All comparison booleans were true. This supplements the harness's separate live certificate and committed/query transaction checks; it does not substitute for them. Observer output: `.logs/controller-atomic-independent-ack.json`.

Source links: [bundle validation and checkpoints](../tools/controller-atomic/atomic.go.tmpl), [isolated controller/chart patch](../tools/controller-atomic/build.py), [six runtime fault cases and acknowledgement checks](../tools/controller-atomic/qualify.py), [redacted evidence exporter](../tools/controller-atomic/evidence.py), [targeted atomic tests](../tools/controller-atomic/atomic_test.go.tmpl).

Final runtime evidence: `.logs/controller-atomic-kind.json`, SHA256 `88ba04379f503f7dafeeb6b74b84ca1438b528d71c06f4b1e26f3c5f0868bffa`. Independent acknowledgement output SHA256 `5ba7cf3989cd03029d08fa9a8b4d837d034db94896679f91e9398b0a5968f76f`. Final log: `.logs/controller-atomic-final-awake.log`. The post-commit manifest `.logs/controller-atomic-evidence.json` records source revision/clean state, source hash, matching patch/binary inputs and tested image.

The preceding resumed attempt is retained as `.logs/controller-atomic-resume-interrupted.json` and remains FAIL after a Kubernetes observation timeout. A nominal 40-second tool wait spanned about 52 minutes; host interruption is a possible cause, not a proven controller defect. The final attempt used a reversible thread-scoped Windows keep-awake request; controller logic, kubelet backoff and qualification criteria were unchanged. That attempt completed all checks and cleanup.

Remaining Milestone 2 priorities: (1) owner-approved incident assessment and migration for original-controller historical key exposure; (2) production issuer/runner integration and effective namespace/CNI/admission containment, with no shared-namespace tenant claim; (3) broader signing/operations/admin/CA/root/Raft identity lifecycle, expiry and recovery; (4) ownership-bounded immutable-bundle retention/garbage collection and backup/restore qualification. AWS authorization and actual EKS qualification remain a later separate gate.
