# Sprint 4 dynamic local Fabric MVP acceptance

5-6 November 2026. **Mandatory local MVP acceptance PASS. Production release BLOCKED.** Sprint 3 remains **LOCAL QUALIFICATION PASSED / AWS EKS QUALIFICATION PENDING**. No AWS access, paid infrastructure or retained-network mutation occurred.

Backend: `e61e216ef2e720048d29ccba72f6f729b1d3580a`. Console: `c4926fb6ac3ec9fa6abb087df488bc01baae7af1`. [Complete evidence](evidence/sprint4-dynamic-final/summary.json) includes exact run dates, immutable image identities, checks, source binding, transactions and cleanup. Raw earlier/failed evidence is preserved separately; completed tests were not restarted after the overnight interruption.

## Implemented and verified

One configuration-driven protected engine consumes customer organizations/MSPs, independently named peers/orderers, placement/resources/storage, combined or split signing/TLS authorities, channels/anchors and approved chaincode artifacts/endorsement thresholds. Customer fixture names do not occur in execution source. The fixed topology remains a regression fixture.

The API persists previous/desired intent and actor/project/parent/revision/target-bound approval before dispatch. Retry and API restart adopt owned Jobs and immutable material. Raft admission is serialized with activation/catch-up before the next member; committed channel changes recover forward. Existing identities, PVCs and previous code services are preserved. Actual upgrades deploy a different approved binary/artifact and package, independently checking executable version and committed policy.

The dashboard imports/edits public customer JSON, discovers approved inventory/artifacts, saves revisions, reviews and applies exact changes, restores server-owned operations after refresh, retries recorded failures and displays per-channel transaction/ledger evidence. Observations are labelled recorded, not fabricated live metrics.

| Gate | Result |
| --- | --- |
| Fixed baseline plus peer addition, definition upgrade, UID-checked restart, new transactions | PASS; three-peer heights 7, 11, 12, 13 |
| A: 2 org / 2 peer / 2 CA / 3 Raft initial; then independent peer, organization and Raft/channel/code changes | PASS; expanded 3 org / 4 peer / 3 CA / 5 Raft / 2 channels |
| B: 3 org / 4 peer / 3 CA / 3 Raft distributed among organizations / 2 channels | PASS |
| C: 2 org / 3 peer / 3 CA / 5 Raft / 1 channel, split signing/TLS authorities | PASS |
| Live Edge dashboard, provisioning, refresh, reviewed expansion, code upgrade, denials and mobile layout | PASS; 9 checks |
| Runtime TLS, public resources, Helm history, logs/arguments, ownership/RBAC and effective Calico isolation | PASS; 15 checks per API topology/baseline; browser inspection passed |
| Final PostgreSQL authorization/recovery/RLS regressions | PASS; 42 events, one explicit production-TLS fixture skip |
| Go / helper / console / Python regressions | PASS; 348 / 8 / 25 / 19 test events; vet/build/format passed |
| Cleanup and retained readiness | PASS; only retained kind cluster remains, 8/8 ready read-only |

## Committed ledger evidence

Every listed channel's peers returned the exact new value and equal ledger height/block hash. Full hashes and approvals are in the raw receipts.

| Case | Channel | Membership | Peer height | Transaction |
| --- | --- | --- | --- | --- |
| A | shipments | 4 peers / 5 orderers | 18 | `030d90f9eaa94e17c5e563357fbf69b9afe268476561e704e6c4f7a70d06d14d` |
| A | customer-added-ledger | 4 peers / 5 orderers | 6 | `f333dc718ad1c757b2cd5353fcbba0e6476b46961f468437209aa4e3d1d55be5` |
| B | supply | 4 peers / 3 orderers | 7 | `e7812604ca88e506e6d78f9a18db5f36361730a3e8ea8330d97cd2003503d78a` |
| B | audit | 4 peers / 3 orderers | 7 | `51bebddcde8fa37dcb613da5d294cceab261bc02f9dcbb3cc602fefd4ab6739e` |
| C | settlements | 3 peers / 5 orderers | 6 | `58da2aa5555821b54741f5f8804050a0d17b3363f4b71c6e92907f04fb3ff10f` |

A's three independently approved changes preserved existing workload UIDs, survived API restart and verified transactions after each stage. Only the final stage changed executable/package identity. Browser expansion independently reached heights 12 and 6 with executable 2.0 on four peers and five consenters.

## Resolved failure

The added baseline peer answered a query at 17:00:38.031 UTC with only genesis committed; blocks 1-6 arrived starting 17:00:38.142 UTC. Its error was `chaincode smp_smoke not found`. [Filtered public timeline](evidence/sprint4-dynamic-final/peer-catchup-root-cause.json) establishes delivery lag, not a reason to weaken endorsement. The worker now waits for authenticated all-peer height/hash convergence after canonical joins. Query/trust errors remain terminal and equal height on different hashes is rejected. Regression tests and the fresh full lifecycle passed.

## Limits and open release gates

This acceptance covers explicit local-development Fabric 2.5.15 Raft/V2_0 on Docker-hosted single-node kind. It does not qualify physical multi-server deployment, EKS/IRSA/AWS CNI/EBS, production authentication or disaster recovery. Real Okta MFA/login and historical exposed-key remediation still need their existing owner actions/evidence. Existing six-case peer-TLS atomic recovery evidence is preserved; it does not qualify every Fabric identity rotation.

BFT, removals, private collections, signing-root/storage/placement replacement and dynamic API deletion remain unqualified and fail closed. Executables must be server-approved immutable artifacts; arbitrary executable templates/images are not accepted. Customer self-service tenant creation and production backup/restore remain future work. These limits must remain visible; this is a local Raft MVP, not a production-readiness claim.

Sprint 5 is authorized to begin from the local operations/recovery backlog, with read-only workload troubleshooting as its first increment. AWS and production stay disabled.
