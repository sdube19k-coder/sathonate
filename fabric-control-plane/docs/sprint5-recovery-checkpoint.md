# Current resume status — 7 November 2026

**Sprint 5 PARTIAL / runtime BLOCKED.** Docker Linux-engine pipe is missing. Do not retry runtime commands until Docker Desktop is running. Latest fixture `dynamic-browser-s5-20261106-225232` stopped without a browser result, per-node progress or final cleanup receipt. Its runtime file identifies `smp-sprint1-4fd47f55`; cached configuration is not proof the cluster is currently running. Neither cleanup nor retained readiness can be claimed current while the daemon is unavailable.

Fresh local Go/vet, console 26 tests/build/format, Python 19 tests and qualification-harness syntax checks passed. See [current machine report](evidence/sprint5-recovery-partial/summary.json). The independently verified monitoring-history fix is committed as `4d8c691`; recovery implementation remains uncommitted until final acceptance.

From normal Windows PowerShell, after Docker Desktop reports Linux engine running:

```powershell
& 'E:/Sathonate/sathonate/fabric-control-plane/tools/sprint5/resume-local.ps1'
```

The script checks Docker once, verifies the exact interrupted cluster provider ID before removing that disposable cluster, refuses to run alongside other unresolved disposable fixtures, reruns serial PostgreSQL regressions, builds the current API and executes recovery-focused API/dashboard acceptance with Calico. It does not run Sprint 4 A/B/C or access AWS. The script passed PowerShell parsing; its complete execution is blocked by Docker. Inspect runtime receipt and verify cleanup after it completes. Do not reuse old native exec session IDs.

Remaining mandatory gates: successful Raft and CA recovery, final-source peer confirmation, complete security inspection and cleanup. Broader documented local backlog also includes safe dynamic-network deletion and consistent encrypted Fabric/CA/PostgreSQL backup/restore; neither is implemented/qualified. The peer-only receipt preserves transaction `70f8043163803ec4ffa961fb312789b5fadc37d3e1c4f1c563374a66aee29ea2`, both peers at height 6 with equal block hashes. This does not qualify orderer/CA recovery or disaster recovery.

## Historical checkpoints (superseded for current process state)

# Sprint 5 recovery qualification checkpoint

6 November 2026. **IN PROGRESS â€” live acceptance pending.**

Workspace: `E:/Sathonate/sathonate`. Preserve Sprint 4 commit `52fcae1` and Sprint 5 first increment `73ae20d`. No AWS or original-controller changes. Retained network is read-only, eight ready deployments and seven bound PVCs.

Implemented, currently uncommitted: complete configured component inventory and storage observations even when pods are absent; allowlisted actionable alerts; deduplicated PostgreSQL monitoring checkpoints; actor/target/template/UID/material-bound node recovery preview and approval; idempotent rollout marker and immutable durable retry input; Raft remaining-quorum refusal; authenticated dashboard review/apply and fresh transaction verification through the existing worker.

Local tests: Go 360 PASS / 21 explicit skips, vet PASS, PostgreSQL 46 PASS / one production TLS fixture skip, console 26 PASS/build/format, Python isolation/evidence 6 PASS and atomic harness 13 PASS. Skips are not runtime acceptance. PostgreSQL recovery tests prove approval durability across manager reopen and actor/node substitution refusal; they do not prove live Fabric recovery.

Current live fixture: `fabric-console/.logs/dynamic-browser-s5-20261106-194423`, disposable kind `smp-sprint1-1efaca03`, Calico enforced fixture. Native exec session 48213. Check process/evidence before restarting. The previous image-name fixture failure removed its cluster. The interrupted 17:44 attempt left `smp-sprint1-7dd7fb2f`; its exact provider ID was verified and it was deleted before this run. Neither attempt is PASS.

Do not mark Sprint 5 complete before mandatory live monitoring/recovery/transaction/security acceptance. Consistent live encrypted ledger/CA/PostgreSQL backup/restore and previewed dynamic network deletion from the broader local operations backlog remain unimplemented/unqualified. Existing encryption primitives and legacy cloud cleanup do not qualify those workflows. Real Okta, historical-key remediation, EKS and physical multi-server qualification remain open production gates.


## Actual 19:42 failure and fix

Run `dynamic-browser-s5-20261106-192911` passed initial provisioning/transaction and expanded organization/peer/Raft/channel/actual-code-upgrade checks, then failed its first workload refresh with HTTP 503. It is NOT Sprint 5 acceptance. Its cluster was removed. Supplemental live correct-issuer/hostname and wrong-issuer/hostname probes passed for both original peers; the tokenless observer was removed.

Exact cause: `Manager.Workloads` tried to append a monitoring checkpoint using `operations.Store.Event`, whose execution mutation path correctly rejects terminal operations. The new `Store.Observe` records only typed, tenant-checked, deduplicated observations under the store lock, without changing status, stages or finished time. It preserves terminal execution guards. A durable file-store test and PostgreSQL HTTP test on a terminal operation passed. Rebuilt API is used by the current fixture. Full Go/vet and serial PostgreSQL passed after the fix. Do not loosen terminal-state mutation rules to repair monitoring.


## Recovery-focused follow-up failures (preserved)

- `dynamic-browser-s5-20261106-194423`: live monitoring/authorization and pending-storage refusal passed; peer recovery preflight rejected the operator's blank Helm release-namespace annotation. Fix: check actual Deployment namespace, owned CR, Helm labels/release name and deployed namespace-scoped Helm storage metadata; reject an explicitly foreign annotation. No operator change.
- `dynamic-browser-s5-20261106-200223`: peer preflight/rollout succeeded but checkpoint emission skipped the worker identity stage. Fix: host recovery checkpoints the existing validation stage; normal worker stage ordering remains mandatory. Added a regression using the actual operations store.
- `dynamic-browser-s5-20261106-223843`: peer recovery/transactions/ledger/PVC/UID checks passed, and missing-quorum refusal passed. Orderer preflight rejected the chart's intentionally empty client-trust placeholder. Fix: allow only that named placeholder when the unchanged ConfigMap explicitly disables client-auth and retains qualified server, cluster and admin TLS/trust; bind ConfigMap UID/version in the plan. Every other missing/empty required material still fails. Dedicated trust-condition negative tests added. Successful peer evidence independently preserved from its tenant-scoped PostgreSQL result. Full acceptance was NOT PASS; cluster removed.

Current fixture was launched at 22:52 with the rebuilt API and optional recovery-focused topology (two organizations/peers/CAs, three Raft orderers, one channel). It saves per-node progress as IN_PROGRESS before final acceptance so later failures do not erase completed cases. Check latest `fabric-console/.logs/dynamic-browser-s5-*` and exec session 22555. Do not repeat completed Sprint 4 A/B/C or baseline tests; no such repetition is required for these host API fixes.
