# 09 · Deployment and recovery

[Handbook](README.md) · [Runbook triage](11-troubleshooting.md) · [Evidence](12-evidence.md)

| Workflow | Steps and safety |
|---|---|
| Customer API and console deployment | Render both Helm profiles first. Use digest-pinned images, external PostgreSQL, a DSN/CA Secret, approved TLS origin and exact ingress/scraper selectors. Apply schema administratively and verify role grants before application startup. Production runner stays disabled. |
| Local Fabric qualification | Use the existing disposable harness and its capacity gates. Supply explicit fixture contexts and pinned controller/worker images. Verify runtime, committed transactions, cross-peer state and cleanup. The retained cluster is outside fixture scope. |
| Monitoring | Inspect API health and readiness on the separate operations listener. Scrape only from approved network identities. Validate rules and actual delivery before operational reliance. Process metrics do not replace ledger or workload health. |
| Scheduled encrypted backup | Approve exact artifact location and target, verify lock and executor lease, stop writers, capture and seal, resume, verify digest and apply bounded retention. A failed slot blocks subsequent schedule work until reviewed. |
| Cold restore | Review exact artifact, binding and namespace/PVC UIDs. Prepare the restore window, verify divergence protection, restore eligible data, resume, compare ledger and commit fresh transactions. Replacement-target cutover needs a separately qualified adapter and single-writer fence. |
| Upgrade and rollback | Check approved image edge and fresh per-channel quorum, backup and storage observations. CA changes require paused enrollment. Inspect UID/resource-version patch guards. Production actuator is absent. After committed trust/config changes, prefer the approved forward procedure when rollback would violate consensus or identity. |
| Retained-data retirement | Preview exact resource identities, authorize deletion, fence the controller and remove only owned workloads. Verify durable acknowledgement and preservation of retained PVCs and Secrets. |

**Before touching a target.** Verify cluster/namespace identity, approval, intended effect, source revision, immutable image digests, backups, and stop conditions. Avoid broad deletion and command output that contains Secret bodies. Record timestamps, redacted receipts, UIDs, ledger heights and transaction IDs as appropriate.

[Helm profiles](../../deploy/charts/sathonate/profiles) · [Helm checks](../../tools/sprint8/check_helm.py) · [Cold backup CLI](../../cmd/fabric-cold-backup/main.go) · [PostgreSQL backup CLI](../../cmd/fabric-postgres-backup/main.go) · [Retained deletion](../../internal/deployment/local_deletion.go) · [Rollout](../../internal/rollout/upgrade.go) · [RC1 scenarios](../rc1-readiness.md).

## Detailed operator recovery checkpoints

1. **Before capture.** Confirm the target binding, approved network and storage identity. Record a peer-readable baseline and a committed transaction. Check that the approved artifact location has sufficient capacity and key custody is independent of the artifact.
2. **After capture.** Keep the authenticated artifact and receipt separately. Verify hash and durable checkpoint. Resume the original workloads and check transactions. An interrupted schedule requires inspection of its lock and checkpoint before another attempt.
3. **Before restore.** Verify the artifact against the intended operation and original bound target. Prepare the fence, prevent conflicting writers and check whether observed control rows or volume contents have diverged. Stop if evidence is ambiguous.
4. **After restore.** Compare the earlier committed value, peer ledger heights and block hashes. Submit a new transaction and independently read it on the required peers. Confirm service state and safe cleanup. A successful original-volume restore cannot establish a replacement-cluster cutover.
5. **After any irreversible channel change.** Preserve exact committed config and voter state. Keep the single writer. Use the reviewed continuation for trust or consenters. Never substitute an old key bundle or roll back Kubernetes alone when the ledger already contains a new channel configuration.

Detailed operator sequence and scope are in [Sprint 5 acceptance](../sprint5-acceptance.md), [Sprint 7 acceptance](../sprint7-acceptance.md) and [RC1](../rc1-readiness.md).
