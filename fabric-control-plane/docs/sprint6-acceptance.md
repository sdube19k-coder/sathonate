# Sprint 6: local multi-node deployment and worker recovery

8 November 2026. **PARTIAL — bounded worker-failure harness implemented and regression-tested; live qualification remains capacity-blocked.** [Current evidence](evidence/sprint6-worker-harness/summary.json) preserves memory observations, the actual gated attempt, targeted tests and read-only retained health. Earlier [Sprint 6 preparation evidence](evidence/sprint6-preflight/summary.json) and all Sprint 4/5 receipts remain unchanged.

## Latest resume — 8 November, 09:25 IST

The unchanged committed gate was rechecked and failed. [Resume evidence](evidence/sprint6-resume-20261108-092455/summary.json) and [exact memory values](evidence/sprint6-resume-20261108-092455/memory-audit.json):

| Metric | Actual bytes | Actual GiB | Required GiB | Result |
| --- | ---: | ---: | ---: | --- |
| Docker total | 6,210,375,680 | 5.78 | 8 | FAIL |
| Docker available | 4,022,329,344 | 3.75 | 6 | FAIL |
| Windows available (gate sample) | 3,454,947,328 | 3.22 | 8 | FAIL |

The explicit WSL cap is still `memory=6GB`; its configuration checksum is unchanged. No system settings were altered and runtime testing stopped before creation. All live gates remain NOT EXECUTED and deployment/expansion/recovery durations remain unmeasured. No completed implementation or regression suites were repeated. Fresh read-only inventory still shows only the retained kind cluster, eight ready deployments and seven Bound PVCs. No cleanup was necessary because nothing was created.

## Current gates

| Gate | Result | Evidence / limitation |
| --- | --- | --- |
| Inspect Windows / WSL2 / Docker limits without changing settings | PASS | [Initial audit](evidence/sprint6-worker-harness/memory-audit-initial.json), [final audit](evidence/sprint6-worker-harness/memory-audit-final.json); `.wslconfig` checksum unchanged |
| Safe capacity for four additional kind nodes | FAIL / BLOCKED | Explicit WSL `memory=6GB`; Docker about 5.78 GiB total / 3.75 GiB available; latest Windows available RAM about 3.22 GiB |
| Worker-pause/rescheduling/recovery harness implementation | PASS source/targeted regression; PARTIAL runtime | 45 Sprint 6 Python tests and one existing default isolation fixture test pass; no live Fabric case executed |
| Dynamic initial provisioning and peer/org/CA/Raft expansion across workers | PARTIAL — NOT EXECUTED | Existing API fixture and generic placement/expansion implementation preserved; memory gate stopped before creation |
| Directed cross-worker DNS/TLS/MSP and Calico enforcement | PARTIAL — NOT EXECUTED | Actual probe placement and mTLS checks implemented; mocked fixture tests are not enforcement proof |
| Worker failure, Raft surviving-majority observation, stateless rescheduling and original-local-PV recovery | PARTIAL — NOT EXECUTED | Bounded case is wired into the live runner; its resource/ownership guards and failure cleanup are regression-tested |
| Monitoring, authorization, tenant isolation and idempotency | PARTIAL multi-node runtime | Checks implemented in the new case; previous Go/PostgreSQL regressions preserved, not relabelled as live acceptance |
| Deployment / expansion / recovery timings and transactions | NOT MEASURED / NOT EXECUTED | No Sprint 6 transaction IDs, ledger heights or duration values exist; none are invented |
| Retained safety and disposable cleanup | PASS read-only | Only `smp-qualification` exists; 8/8 ready, seven Bound PVCs; no new Docker/Kubernetes resources were created |

## Memory determination

The host has approximately 15.75 GiB physical RAM and 20 logical CPUs. `%USERPROFILE%/.wslconfig` explicitly sets `[wsl2] memory=6GB`. Docker is the local Linux WSL2 engine, with about 5.78 GiB usable RAM. This cannot meet the unchanged **8-GiB Docker total / 6-GiB Docker available** gates under current settings. Installed RAM alone does not establish usable headroom.

The host-available gate was **strengthened from 2 to 8 GiB**, reserving the conservative new-fixture allocation of 6 GiB plus 2 GiB for Windows/host margin. Free guest virtual pages need not already be resident physical Windows RAM; raising the VM cap alone is not proof that Windows can absorb the fixture peak. This is a conservative safety policy, not a measured Fabric/vendor minimum. Samples varied as other host workloads changed; the earlier final sample had about 3.42 GiB Windows-available RAM (latest resume: 3.22 GiB), still below that reserve.

The reservation for the new fixture remains 2.5 GiB Kubernetes/CNI + 2 GiB Fabric requests + 1 GiB worker peak + 0.5 GiB headroom. CPU and backing disk capacity passed their gates. No WSL/Docker settings, other applications, caches or retained resources were changed or reclaimed. The before/after WSL configuration checksum is identical.

A WSL limit change may require restarting WSL to take effect ([Microsoft configuration documentation](https://learn.microsoft.com/en-us/windows/wsl/wsl-config)). That could interrupt the retained cluster and was not performed. A nominal `memory=8GB` setting also must not be assumed to provide 8 GiB usable Docker memory: the current six-GiB cap has kernel overhead. Any owner-managed change must be validated using actual engine and physical-headroom measurements afterward.

The actual [runner result](evidence/sprint6-worker-harness/entrypoint-blocked.json) stopped before database credential access, kind creation, image import or any Kubernetes mutation. The four-node plan remains **one control-plane plus three workers with Calico**; no topology was deployed.

## Completed harness

The runner reuses the existing generic application-controlled engine and approved worker/controller. Its customer fixture initially contains two organizations, two peers, two CAs, three Raft orderers and one channel, then expands through the API to three organizations, four peers, three CAs, five Raft orderers and two channels. These are regression inputs, not production topology templates. Worker/organization/channel names are resolved from recorded customer configuration and actual observations. Existing placements remain unchanged during expansion; all kind nodes retain the physical `local-host` failure domain.

[Worker state machine](../tools/sprint6/worker_failure.py) and [live adapter](../tools/sprint6/worker_live.py):

1. Bind the immutable protected configuration, namespace ownership, original node/container identities, PVC/PV UID and claim/host affinity, and immutable issuer Secret hashes. Only the loopback `fabric_smp_tests` database is accepted. Fresh local-development tokens are cryptographically generated; only hashes are stored in a protected server-grant file. The native API listener must belong to the expected process before any token is sent.
2. Verify server-owned reads, missing/invalid token rejection, cross-tenant/project denial, viewer write denial and stale approval rejection. Select an actual worker whose loss leaves a numerical majority for **every** customer channel and exercises peer/CA/orderer placement. No endorsement, MSP or trust policy is changed.
3. Pause only that exact owned worker container ID, never the control-plane or retained cluster. Record pause intent before acknowledgement. An independent polling watchdog requests unpause at 180 seconds. Lost acknowledgements, failures and timeout cleanup cannot become PASS.
4. Observe Node NotReady, authenticated degraded monitoring and its durable history checkpoint without reopening the completed operation. Observe a tokenless stateless canary rescheduled to a different healthy worker. A separate read-only pod mounting an original local PVC, constrained to other workers, must remain unschedulable specifically because of volume node affinity. No stateful pod/PVC is force-deleted or rebound.
5. Query surviving orderers through an observer on another worker using the existing immutable channel TLS identity, mounted read-only. Verify issuer/hostname/TLS and client authentication, active consenter membership, recorded ledger height and a surviving majority per channel. Calico's positive and negative controls are additionally pinned to different workers and actual placement is checked.
6. Resume the same container, verify original node/PVC/PV identities and stateful placement, and check previously committed transactions as VALID on every channel peer. Run reviewed, authenticated API reconciliation with idempotent replay; require distinct fresh committed transactions and equal advancing peer ledgers afterward. Verify immutable identity bytes, restored monitoring and no redundant reconcile restart.
7. Delete only acknowledged helper UIDs, stop the owned native API and remove its protected hashed-grant file. Native logs are redacted before disk; any detected credential emission still fails acceptance. Cleanup and resume failures are preserved in receipts and cannot produce PASS. Outer teardown validates all owned nodes, unpauses owned paused containers after child failure/timeout, and preserves a final report even when cleanup fails.

Atomic progress receipts retain the failed boundary, checks and cleanup outcome. Initial deployment timing remains explicitly labelled as including its deliberate retry; expansion stages have separate timings. The new case measures stateless rescheduling, node return and recovery through fresh transactions separately. None of those runtime timings has been measured here.

## Verification actually performed

- **45 Sprint 6 targeted Python tests PASS**, including all state-machine failure boundaries, lost pause acknowledgement, cleanup refusal, PV/claim/host replacement, channel-quorum selection, credential redaction, listener ownership, stale/foreign resource protections, and aggregate acceptance gates.
- **One existing single-node/default isolation fixture regression PASS**. Cross-worker isolation tests use mocked commands and do not prove live CNI enforcement.
- Python static checks and PowerShell syntax PASS. Resource warnings are treated as errors in the targeted suite; the existing archive loader's file handle was corrected to close explicitly.
- A real temporary Windows loopback socket verified listener/PID attestation. This is neither real API login nor Kubernetes/Fabric acceptance.
- Production Go and frontend source are unchanged. The previous **379 Go passes / 23 skips and 47 PostgreSQL passes / one skip** were preserved instead of unnecessarily rerun. Relevant Go source hashes still match the committed prior manifest.
- The normal PowerShell runner was invoked and rejected actual capacity. No four-node cluster, live fault, new Fabric transaction or ledger comparison ran. Existing Sprint 4/5 success evidence was not reused as proof of Sprint 6 runtime success.

[Targeted test evidence](evidence/sprint6-worker-harness/regressions.json) records exact scopes and checksums. Earlier failed fixture evidence remains preserved. Development test/formatting corrections are not presented as live network failures or acceptance.

## Explicit boundaries

This case covers bounded process suspension/unreachability and recovery of the **same** kind worker. It does not simulate disk loss, hard power loss, worker replacement, long outages beyond ordinary stateful eviction, or every crash boundary. The watchdog is independent of observation polling, not an independent physical machine; Docker/host failure can prevent resume and must produce failed/unverified cleanup.

Local-path storage does not migrate between workers. The negative read-only PVC probe is distinct from stateless rescheduling. Original PVC/PV identities, affinity and ledger state must survive return of the original worker. Replacement-PVC or replacement-cluster disaster recovery is unsupported.

No writes are submitted during the outage. The case checks surviving mTLS consenter membership/height, preserved committed history and new transactions **after recovery**. It does not prove leader-election latency, uninterrupted writes under failure or every endorsement policy's availability. A single-PC kind cluster does not establish independent physical-host resilience. Native listener attestation currently targets Windows; unsupported host platforms fail closed.

## Next steps and closure

1. Owner-managed memory/headroom changes must preserve the retained network; no restart is authorized by this execution. Actual Docker total must reach 8 GiB, Docker available 6 GiB and Windows available 8 GiB under the conservative reserve. Releasing other host workloads or changing a WSL cap is an owner decision; neither was done automatically.
2. From `fabric-control-plane`, inspect without changing settings:

   ```powershell
   python tools/sprint6/memory_audit.py --output .logs/sprint6-memory-audit-new.json
   ```

3. Once all gates pass, run the complete prepared bounded suite:

   ```powershell
   & ./tools/sprint6/qualify-local.ps1
   ```

   Logs and atomic progress receipts appear under `.logs/dynamic-sprint6-*`. The runner now includes worker pause/rescheduling/storage/recovery after initial provisioning and expansion; it reports bounded acceptance only if all live checks, security inspections and cleanup pass.
4. Fix any actual runtime failures, preserve their receipts, and update the gate matrix from observed transactions, heights, placements and timings. Do not close Sprint 6 on unit tests alone.

**Sprint 6 remains ineligible for closure.** Real Okta/MFA, historical-key remediation, AWS/EKS/IRSA/CNI/EBS, production storage/backup and multi-server release gates remain open. No AWS/GCP/Azure or paid infrastructure was accessed.

Harness implementation commit: `ce322b9a22fef83fc05c475799f53c48e4bbf94f`. [Canonical source hashes](evidence/sprint6-worker-harness/SOURCE_SHA256SUMS) normalize CRLF to LF. This source revision has targeted regression verification, not live multi-node acceptance.
