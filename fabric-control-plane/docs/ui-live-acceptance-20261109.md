# Local UI lifecycle acceptance — 9 November 2026

**Core customer lifecycle PASS; full UI qualification PARTIAL.** [Source-bound receipts](evidence/ui-live-20261109/summary.json) preserve both fixtures and the interrupted case.

Customer-defined creation and reviewed expansion passed on real disposable kind: three organizations, four peers, three CAs, five Raft orderers, two channels, anchor updates and actual chaincode executable/definition upgrade to 2.0. All four peer heights matched: shipments 13; customer-added-ledger 7. Expanded-peer recovery also passed. Windows sleep interrupted the later Raft test; its exact failed Kubernetes request was not captured. That whole fixture remains PARTIAL.

A 17.06-minute smaller fixture then passed browser-driven provisioning, monitoring, peer/Raft/CA recovery, fresh transactions and ledger consistency, authorization, unsafe-storage and quorum guards, idempotency, live TLS issuer/hostname negatives, Calico ingress/egress denial, RBAC and actual credential scans. Administrator-approved retained-data deletion removed 20 compute resources and verified 126 retained objects, original PVCs and unchanged Secret bytes. Stale approvals/UID replacement, foreign workloads and automatic resurrection were rejected. Its whole fixture PASS is recorded separately.

Final recovery transaction: `8f0802a3488fecdd62061cf9f6fbea3c9ca3a74c128547db37d089a1de580eb4`; both peers reached height 8. Expanded-channel transactions and full node evidence are in the receipts. No fake API or Fabric result was used.

Only the retained `smp-qualification` cluster remains. A fresh 9 November read-only check found 8/8 deployments ready and seven Bound PVCs. No retained resource was modified. No cloud resources were accessed.

The separate fixed-profile six-step wizard, its deployment-retry fault case and diagnostic-download path have NOT been rerun on the redesigned UI. Current Windows headroom fell below 1 GiB without a new fixture; no additional cluster was started. Do not claim every implemented screen/action is qualified. Real Okta, historical remediation, multi-node resilience and production release remain open. Cold backup/restore is an offline administrator facility, not a customer UI operation, and was not repeated. Compute deletion is not permanent erasure.

The only harness change adds allowlisted, redacted Kubernetes failure categories; no raw stderr, credentials or public API security behavior changes. Syntax and formatting were checked, and the updated harness completed the successful recovery fixture. Existing current-product regressions remain 379 Go / 45 PostgreSQL / 26 console tests PASS with explicitly recorded skips, vet and build PASS; these were executed on 8 November, not relabelled as fresh 9 November runs. See [regression evidence](evidence/ui-e2e-20261108/summary.json).
