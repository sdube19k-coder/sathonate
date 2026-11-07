# Sprint 5 local operations acceptance

8 November 2026. **PASS — bounded local operations acceptance.** Runtime receipts were captured on 7 November; final regressions and read-only cleanup/retained-health recheck passed on 8 November. This report is bounded to disposable, single-node kind and explicit local-development authentication. AWS and production were not accessed. Sprint 3 remains open with AWS EKS qualification pending; Sprint 4 A/B/C evidence is preserved without rerunning it.

## Acceptance matrix

| Gate | Result | Evidence |
| --- | --- | --- |
| Authenticated monitoring, workload/PVC readiness, actionable fixed diagnostics and history | PASS | [node recovery cases](evidence/sprint5-node-recovery/summary.json); monitoring store increment `4d8c691` |
| Peer restart, Raft restart, CA restart; safe preflight, PVC/identity preservation, unrelated-node preservation, new transactions and matching ledgers | PASS | [three node cases](evidence/sprint5-node-recovery/cases.json), committed `dd0a913`; not repeated |
| Encrypted cold capture, seven original-PVC restorations, two missing captured peer identities and missing scoped control rows | PASS, bounded original target | [cold receipts](evidence/sprint5-cold-accepted/cold-volume-recovery.json) |
| Prior ledger state survives restoration; fresh authenticated API transaction afterward | PASS | Prior tx `e92089065e182f39176346542f4672991defe0c80013ffa8fcbb92db8de43928` VALID on both peers; fresh tx `04374547c9645c12d6ee497c767babab85b78dd3b58ae16c892f428a91fc24e7`, both height 6, equal block hashes |
| Live certificate trust, TLS hostname/issuer rejection, RBAC, actual Calico isolation and credential inspection | PASS | [cold fixture checks](evidence/sprint5-cold-accepted/summary.json); current deletion run repeats applicable checks |
| Retained-data deletion through browser approval, stale UID/digest refusal, tenant/operator isolation and retirement fence | PASS | [final browser deletion](evidence/sprint5-final/deletion-browser.json): 17 exact resources removed, 126 retained objects verified, all original PVCs Bound and Secret bytes unchanged |
| Disposable cleanup and final serial regressions | PASS | [final regressions](evidence/sprint5-final/regressions.json), [cleanup and retained health](evidence/sprint5-final/cleanup-retained.json) |

## Implemented behavior

[Node recovery](../internal/deployment/node_recovery.go) binds server-owned tenant/project, actor, original target, Deployment/CR/pod/PVC UIDs, complete approved template and material versions to an immutable preview. Other Raft members must retain quorum. Retry revalidates the original intent and avoids a redundant restart. [Monitoring](../internal/deployment/workloads.go) reports fixed typed observations without payloads or raw Kubernetes errors; durable observations do not reopen completed operations.

[Retained deletion](../internal/deployment/local_deletion.go) journals approval before mutation, stops the controller first, applies exact UID preconditions and fences the namespace against automatic execution. Namespace, Fabric CRs, identities, Helm history, PVCs and containment remain. CR finalizers are deliberately avoided because Helm uninstall can delete storage. This is compute removal with retained data, not permanent erasure. Durable API retries require the original actor and approval.

[Cold maintenance](../internal/deployment/cold_backup.go) is an offline administrator utility, restricted to the protected disposable target and loopback `fabric_smp_tests`. It holds the API executor lock, quiesces writers, encrypts the bounded artifact with a separate protected key, validates archive safety, restores exact bytes/owners to original empty PVCs and resumes the controller last. It restores only missing captured identities and missing scoped network rows; it never imports memberships, database roles or schema, nor overwrites divergent data. [Operating boundaries](sprint5-local-maintenance.md) are mandatory.

## Failures preserved and fixes verified

1. All three node cases passed before their browser report writer encountered `cfg is not defined`. The scope bug was fixed; the cases remain separately labelled rather than a whole-fixture PASS.
2. Earlier cold restoration reached ledger verification but the restarted API rejected an expired negative-test grant. The harness now removes expired fixture entries without extending valid tokens or changing backend authentication; two fixture-auth tests pass. [Previous outcome](evidence/sprint5-cold-pretransaction/summary.json) remains PARTIAL.
3. The successful cold rerun failed subsequent browser deletion acknowledgement. Vite recorded `socket hang up`: cleanup allowed five minutes while the native server write deadline was 150 seconds. Only the authenticated local cleanup route now extends its response deadline to its bounded context plus ten seconds. A [real HTTP regression](../internal/httpapi/cleanup_transport_test.go) proves a response survives the ordinary deadline. The final deletion-only run `dynamic-browser-s5-20261107-195129` passed with a durable read-back and idempotent acknowledgement.

The earlier intermittent unclassified channel-fetch failure was not reproduced by the successful node cases. Diagnostic categories do not prove a causal fix; longer production recovery soak remains unqualified.

4. Extracting deletion-only qualification initially left its API helper out of scope. This harness-only failure occurred before browser deletion and cleaned up automatically; [receipt](evidence/sprint5-deletion-harness-failure/summary.json) remains. A shared authenticated transport, four operations-harness tests and static checks corrected it before the successful run.

## Final verification

The [final manifest](evidence/sprint5-final/summary.json) links all accepted receipts and distinguishes preserved tests from the deletion-only rerun. Final Go regressions: 372 passed, 23 opt-in skips; PostgreSQL: 47 passed, one gated skip; console: 26 passed; Python: 23 passed across atomic/isolation/operations harnesses; vet, TypeScript/build, formatting and Python static checks passed. The existing large console chunk warning remains non-blocking. The separate real cold-helper roundtrip/UID test passed. Opt-in skips are not counted as successes.

Runtime credential checks decoded all 12 Helm history records, inspected public CRs/ConfigMaps/workload arguments and logs, and compared actual private values against source, evidence and built assets. Live certificate mismatch rejection and enforced cross-namespace Calico denials passed. The final whole deletion fixture PASS does not relabel earlier failed fixtures.

Only `smp-qualification` remains in kind. Its fresh read-only check found eight ready deployments and seven Bound PVCs. Temporary fixture processes, runtime configuration and encrypted artifact/key directory are gone. No retained-network mutation, AWS or production operation occurred. Original controller and Sprint 4's qualified worker remain unchanged; the separate diagnostic worker's 39 compiled local files match its build receipt.

[Runtime source hashes](evidence/sprint5-final/SOURCE_SHA256SUMS) normalize CRLF to LF. The only production Go delta after cold acceptance is the cleanup HTTP deadline fix; no cold restoration or worker logic changed. The final source manifest and implementation commit are recorded with the evidence.

## Limitations and open release gates

- No replacement cluster, namespace or PVC restoration; no full PostgreSQL server/schema/role disaster recovery; no qualification of every partial snapshot/restore crash boundary. Artifacts are limited to 16 MiB plaintext and 128 MiB expanded per volume. The test artifact/key are removed after acceptance; this is not an ongoing backup service.
- No permanent erasure or customer reactivation of retired networks. A retained-data fence is intentionally not auto-cleared.
- Single-node kind qualification only; actual multi-server storage/recovery remains untested.
- Real Okta/MFA acceptance and historical exposed-key remediation remain open. No retained key material was exported or rotated by these tests.
- AWS EKS/IRSA/CNI/EBS and production release remain unqualified. No cloud resources or paid services were used.

Implementation commit: `f0936f6510c19740f3460a6217f02f10a1fe1894`. Evidence is recorded separately so runtime receipts bind to an immutable source revision.
