# Sprint 6: local multi-node Kubernetes qualification

8 November 2026. **PARTIAL — capacity blocked; no multi-node live acceptance.** The workspace started clean at `0b30021425630738f35902ed72e885ea0d2d93fe`. [Evidence](evidence/sprint6-preflight/summary.json) records the read-only preflight, planned kind configuration, actual entry-point refusal, regressions and retained health. No Sprint 6 resources were created; AWS/GCP/Azure and paid services were not accessed.

## Capacity gate

| Resource | Observed | Gate | Result |
| --- | --- | --- | --- |
| Docker Linux engine | 29.7.2, local named pipe | Local Linux Docker | PASS |
| Docker CPUs | 20 | 4 minimum | PASS |
| Docker RAM | 5.78 GiB total; about 3.85 GiB available | 8 GiB total / 6 GiB available reservation | FAIL — creation blocked |
| Host RAM | About 15.75 GiB installed; 4.5–4.9 GiB available during samples | 2 GiB outside the fixture | PASS |
| Docker backing C: disk | About 45.3 GiB free | 40 GiB free | PASS |
| E: project/cache disk | About 325.6 GiB free | Observed separately; does not substitute for Docker backing disk | PASS |

The memory floor is a **conservative engineering reservation, not a measured Fabric minimum**: 2.5 GiB for four Kubernetes/CNI nodes, 2 GiB Fabric requests, 1 GiB worker peak and 0.5 GiB headroom, plus room outside the fixture for existing workloads. The retained container was using about 1.17 GiB. Running this unqualified workload close to the six-GiB VM limit could jeopardize the retained network. Docker/WSL limits were not changed, restarted or bypassed. No pruning or retained resource reclamation occurred.

The real multi-node entry point refused before database credential access, kind creation, image import or Kubernetes mutation. See [exact metrics](evidence/sprint6-preflight/capacity.json) and [entry-point result](evidence/sprint6-preflight/entrypoint-blocked.json). A preflight PASS would permit testing; it would not establish acceptance.

## Gate matrix

| Gate | Result | Actual evidence / limit |
| --- | --- | --- |
| Four-node kind configuration: 1 control-plane + 3 workers, Calico CNI | PASS for generated plan/unit checks; PARTIAL deployment | [Plan](evidence/sprint6-preflight/planned-kind.json) was not applied |
| Trusted worker inventory/provider binding | PASS unit; PARTIAL runtime | Three exact owned workers; foreign/duplicate/control-plane placements rejected; immutable copy; legacy single-node serialization preserved |
| Customer-defined organization/CA/peer/orderer/channel placements | PASS unit; PARTIAL runtime | Generic resolver tested against customer A/B/C distributed inputs; no production hardcoded Fabric topology added |
| Initial provisioning and peer/org/CA/Raft expansion | PARTIAL — NOT TESTED live | Existing API fixture prepared for three-worker placement and expansion; original node placements carried forward |
| Cross-worker DNS/networking, TLS/MSP, Calico enforcement, scheduling and Raft quorum | PARTIAL — NOT TESTED | Prior single-node Sprint 4/5 results are not multi-node proof |
| Worker outage, pod rescheduling, recovery, PVC preservation, new committed transactions and matching ledgers | PARTIAL — NOT TESTED | Worker-failure acceptance still requires implementation and execution; no outage was injected |
| Monitoring, tenant authorization, idempotency and failure handling | PASS existing unit/PostgreSQL regressions; PARTIAL multi-node acceptance | No multi-node runtime claim |
| Deployment / expansion / recovery timings | PARTIAL — NOT MEASURED | Instrumentation prepared for deployment and individual expansion stages; no durations invented |
| Retained safety and cleanup | PASS read-only | Only `smp-qualification` remains; 8/8 ready, seven Bound PVCs; nothing created to remove |

## Implemented preparation

- [Read-only capacity gate](../tools/sprint6/preflight.py) rejects insufficient/ambiguous metrics and remote Docker endpoints. It renders an explicit four-node plan without creating resources. Tests cover the reservation boundary and every missing/invalid resource metric.
- [Owned worker inventory](../internal/deployment/local_workers.go) adds optional `workerNodes` to protected local configuration. It accepts the three generated worker identities and exact worker-only placement selectors; provider IDs are rechecked alongside the original loopback/kubeconfig/control-plane protections. Readiness stays separate from ownership so an unavailable owned worker can still be investigated. Empty inventory preserves the existing single-node digest/behavior. No RBAC grant or Secret rule was relaxed.
- [Existing fixture loader](../tools/sprint1/qualify.py) now has an explicit Sprint 6 multi-node path behind that capacity gate. It imports/pins approved images on every owned node, validates all node provider IDs, waits for Calico/node readiness and guards partial-create teardown. Legacy single-node behavior remains the default. A mocked-command regression verifies both single-node and four-node image imports/pins; live import remains unqualified.
- [Customer placement fixture](../internal/httpapi/multinode_placement_test.go) spreads initial Raft members across three logical workers and adds only new member placements during staged expansion. It deep-copies input and preserves previous member placements. **All kind workers keep the physical `local-host` failure domain**: they are not independent physical hosts.
- [API acceptance instrumentation](../internal/httpapi/secure_api_e2e_test.go) records actual managed pod-to-worker placement when run, separates initial provisioning (explicitly including its controlled retry) from expansion-stage durations and refuses workload placement outside the trusted inventory. The entry point will still report PARTIAL after those gates until worker-failure acceptance is implemented and qualified.

The planned Fabric input is existing customer topology A: initially two organizations, two peers, two CAs, three Raft orderers and one channel; staged customer expansion reaches three organizations, four peers, three CAs, five Raft orderers and two channels. Existing customer B/C configurations are additionally covered by placement unit tests. These are test fixtures for the generic engine, not new production topology templates. **None was deployed in Sprint 6.** Actual node placement, transaction IDs and ledger heights therefore remain unavailable.

## Tests and preserved failures

Final regressions: **379 Go tests passed / 23 gated skips; 47 PostgreSQL tests passed / one gated skip; seven Sprint 6 Python tests passed; vet, Python static checks and PowerShell syntax passed.** Frontend code is unchanged; no Sprint 6 browser acceptance is claimed. [Machine test summary](evidence/sprint6-preflight/regressions.json) includes log checksums.

A new fixture regression initially found that later expansion stages reset a previously added node's placement. The fixture now copies accepted placements forward without changing production rejection of placement replacement. Caller mutation is also tested. A mocked loader test initially mismatched the `ctr images list` command; the test double was corrected. [Failure record](evidence/sprint6-preflight/failures.json) preserves the distinctions. No live Fabric failure was concealed or relabelled PASS.

Sprint 4 and [Sprint 5](sprint5-acceptance.md) evidence is preserved, including successful node recovery, original-PVC cold restore and retained-data deletion. Their live suites were not unnecessarily rerun. The original controller and retained cluster/PVCs/credentials were not modified.

## Storage and resilience limits

Kind's local-path volumes have node affinity and reside inside a worker container. A stateful pod cannot be assumed to migrate with its data when that worker is stopped. Future acceptance must verify the original PVC/PV UID, claim binding and PV node affinity; observe blocked stateful rescheduling; recover the original worker/storage; and then commit new transactions and compare ledgers. Replacing the worker container or rebinding a PVC is not an approved recovery shortcut. Stateless rescheduling needs its own separate observation.

Five Raft members spread 2/2/1 across logical workers can retain a numerical majority after losing one worker, but that does not establish transaction availability: peer/organization endorsement requirements and storage availability remain authoritative. No policies may be relaxed to obtain a passing transaction. A single-PC kind cluster cannot prove independent physical-host resilience or full disaster recovery.

## Exact next steps

1. Arrange sufficient local Docker memory headroom **without interrupting or altering retained infrastructure**. Increasing a WSL/Docker limit may require a restart; this agent did not perform one. Owner-scheduled maintenance or a separately isolated local engine would require a deliberate owner decision outside this run.
2. Recheck capacity from the backend directory, using a fresh output filename:

   ```powershell
   python tools/sprint6/preflight.py --output .logs/sprint6-preflight-new.json --kind-config .logs/sprint6-planned-new.json
   ```

3. Once the capacity gate passes, run only the prepared multi-node initial/expansion fixture:

   ```powershell
   & ./tools/sprint6/qualify-local.ps1
   ```

   Its logs are under `.logs/dynamic-sprint6-*`. It reuses the protected API/controller/exporter and disposable kind lifecycle, keeps security enforcement enabled and cleans only its owned nodes. This command does **not** complete Sprint 6: worker failure/rescheduling tests remain unimplemented/unqualified and its report says PARTIAL.
4. Add and execute bounded worker-stop/start tests, directed cross-worker DNS/TLS/Calico probes, stateless rescheduling and original-local-PV recovery; capture monitoring/authorization/idempotency evidence and separate recovery durations. Verify new transactions and equal ledger hashes afterward, then teardown and recheck retained health.

Sprint 6 is **not eligible for closure**. Real Okta/MFA, historical-key remediation, AWS/EKS/IRSA/CNI/EBS, production storage/backup and multi-server release gates remain open. No cloud qualification or production readiness is implied.

Implementation/preparation commit: `20181783539fa0e631b0da60adf5a3a6b75e00f9`. The [source manifest](evidence/sprint6-preflight/SOURCE_SHA256SUMS) uses CRLF-to-LF canonical hashes. This revision has unit/regression verification, not multi-node runtime acceptance.
