# Sprint 5: local operations and recovery

8 November 2026. **PASS — bounded local operations acceptance.** See the [acceptance report](sprint5-acceptance.md), [final evidence](evidence/sprint5-final/summary.json) and [maintenance boundaries](sprint5-local-maintenance.md).

Implemented and verified locally:

- Authenticated tenant/project-scoped workload, peer, Raft orderer, CA and PVC observations, fixed actionable diagnostics and deduplicated durable history.
- Reviewed UID/template/material-bound peer, Raft and CA recovery, with original PVCs/identities preserved, unrelated nodes unchanged, new committed transactions and equal peer ledger hashes. Completed cases were preserved without rerunning them.
- Offline encrypted capture and original-PVC cold restore, exact missing captured peer identities and scoped network rows, prior ledger transaction preservation and a new authenticated committed transaction afterward.
- Administrator-previewed retained-data deletion in the dashboard, exact UID preconditions, stale/foreign/unauthorized refusals, controller-first removal, unchanged Secret bytes, original Bound PVCs and a durable retirement fence/acknowledgement.
- Final regressions, actual TLS/RBAC/Calico/credential inspection and disposable cleanup. The retained network was observed read-only at 8/8 ready with seven Bound PVCs.

This acceptance covers single-node disposable kind and explicit local-development authentication. Cold maintenance accepts only the guarded original target and loopback `fabric_smp_tests`; no replacement-cluster/PVC or PostgreSQL-server disaster recovery is qualified. Deletion removes compute and execution authority while retaining data; permanent erasure and reactivation are unsupported. Real Okta, historical-key remediation, AWS EKS and production/multi-server gates remain open. Sprint 4 acceptance is preserved; Sprint 3 remains LOCAL QUALIFICATION PASSED / AWS EKS QUALIFICATION PENDING.
