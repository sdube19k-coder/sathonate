# Sprint 5: local operations and recovery

7 November update: **PARTIAL / runtime BLOCKED**. See [current checkpoint and exact resume command](sprint5-recovery-checkpoint.md) and [redacted evidence](evidence/sprint5-recovery-partial/summary.json). Monitoring and one peer recovery passed in prior disposable runs; final Raft/CA recovery, security inspection and cleanup remain unqualified. Docker Linux-engine pipe is currently unavailable. The section below records the earlier first increment.

6 November 2026. **IN PROGRESS. First read-only workload increment implemented; full Sprint 5 is not complete.** The user selected the existing local operations/recovery backlog. Sprint 4's qualified local MVP is preserved at `52fcae1`, with backend `e61e216` and console `c4926fb`. Its completed acceptance is not rerun unnecessarily or relabelled as Sprint 5 acceptance.

## Implemented increment

Authenticated `GET /api/v1/deployments/{operationId}/workloads` resolves the server-owned tenant/project operation and recorded target. It accepts only the explicit protected local runner, refuses simulation/history/superseded records and reuses target/kubeconfig/provider-ID/namespace ownership checks before scoped Kubernetes GETs. No Pod, Job, PVC, Secret, permissions or network configuration is modified. EKS/production are not enabled.

A typed allowlist returns current managed pod readiness, restart counts, fixed waiting/last-exit categories, referenced PVC phases, missing/unready components and fixed recovery guidance. It excludes environment, arguments, Secret references, annotations, raw status/event/error messages and logs. Both typed API lists and kubectl generic List framing are validated; foreign namespaces and wrong object kinds fail closed. Container readiness cannot bypass the Pod Ready condition.

The customer-configuration dashboard can request this view explicitly. Requests time out and are aborted/discarded when the operation changes. Failed refreshes clear old observations. The view distinguishes current Kubernetes readiness from recorded Fabric ledger verification and never treats ready pods as proof of ledger consistency. Reads do not retry deployment or restart workloads.

## Actual verification and limits

- Full Go regression and vet passed; see [machine evidence](evidence/sprint5-workload-increment/summary.json) for exact counts and skips.
- PostgreSQL-backed HTTP authorization tests passed for read-only success, absent/invalid tokens, cross-tenant/project denials and provider-error redaction. The workload provider in these HTTP tests is a fixture; this is not live HTTP-to-Kubernetes acceptance.
- An explicitly enabled GET-only live projection checked the retained inventory without mutations: eight ready workloads and seven bound PVCs. Raw Kubernetes payloads were kept in memory; only the allowlisted projection was exported. This proves real CLI schema/projection handling, not the new API binding.
- Console unit tests, TypeScript/build and formatting passed. The new widget has not yet undergone live browser acceptance. The nine Sprint 4 browser checks are preserved as Sprint 4 evidence and are not claimed for this widget.

## Next acceptance and recovery work

1. Qualify the new HTTP-to-Kubernetes path and widget against a disposable protected deployment, including degraded/missing workloads, pending/lost claims, revocation, foreign-resource filtering and stale-response handling.
2. Design and implement previewed, ownership/UID-bound safe deletion with explicit persistent-data retention/deletion semantics, then qualify it only in disposable environments.
3. Add encrypted backup/restore and recovery acceptance using fresh disposable identities and storage. Retained-network or historical-key remediation requires the existing explicit owner actions; do not export or rotate retained private material automatically.
4. Keep real Okta, historical-key remediation, EKS/IRSA/CNI/EBS, multi-server and production-release gates open. No AWS or paid resources are authorized for this sprint.
