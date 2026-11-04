# Sprint 4 bounded lifecycle acceptance

4 November 2026. **Verified lifecycle increment PASS; Sprint 4 overall PARTIAL.**
Sprint 3 remains **LOCAL QUALIFICATION PASSED / AWS EKS QUALIFICATION PENDING**.
No AWS access or production deployment occurred. The retained network was
inspected read-only: 8/8 ready deployments and seven Bound claims. Both
disposable qualification clusters were deleted; only `smp-qualification` remains.

Backend commit: `fbdd205`. Console commit: `e023c17`. The
[source-linked manifest](evidence/sprint4-lifecycle-verification.json) records
full commit IDs, source hashes, worker/image identities, original timestamps,
raw evidence hashes, transaction IDs, ledger hashes, checks and cleanup. The
[browser evidence](evidence/sprint4-lifecycle-browser.json) retains its actual
31 October execution date; it is not represented as a 4 November browser run.

## Implemented and executed

The authenticated API and console preview and explicitly approve four bounded
operations on the protected local profile. PostgreSQL persists the approved
intent before execution; retries and acknowledgement replays retain the same
operation, actor, project, parent and target binding. Superseded previews and
base redeployment over expanded membership fail closed. This is an extension of
the existing two-organization/three-Raft profile, not arbitrary provisioning.

| 4 November API operation | Three peer heights | Committed transaction |
| --- | --- | --- |
| Add independently enrolled `org1-peer-2`, join and synchronize | 7 / 7 / 7 | `287128584e9086f9233a069dfd24499d1e10b947c15452e734cdc04be4b7138d` |
| Upgrade approved definition to version 2.0 / sequence 2 | 11 / 11 / 11 | `981da332b9f256c2f3ea0868620def89fd987ad35e68f6ffcfdedd952d5c58ae` |
| UID-checked `org1-peer-1` restart and reconciliation | 12 / 12 / 12 | `c170378fde134bc82c6f548bccce4698f0c7c1f58592e984393534a3a88dde9f` |
| Reconcile and submit another new verification transaction | 13 / 13 / 13 | `316da2c9b794d742b1bd2fb159e1b6a1647ba0d8715225903970d50e9a593fcd` |

All four operations survived an API/application restart and adopted their
recorded worker. Each read back the exact new value on every managed peer and
required equal ledger hashes. The definition upgrade retains the same approved
package/image; actual chaincode code upgrades are unfinished. Both organization
endorsements remain mandatory and the committed policy is decoded and checked.

The endorsement failure's root cause was the new peer's empty gossip bootstrap:
it could not fetch Org1 private lifecycle approval data and remained at height
1. [Bootstrap selection](../internal/kubernetesfabric/gossip.go) now uses other
managed peers in the same organization. Separate
[endorsement construction](../internal/kubernetesfabric/endorsement.go) prevents
duplicate Org1 peer positions from replacing Org2's required endorsement.
[Canonical genesis fetching](../internal/kubernetesfabric/genesis.go) checks the
channel/header/data hash before joining. These protections were qualified with
fresh generated identities; no retained material was rotated.

## Acceptance matrix

| Gate | Result and scope |
| --- | --- |
| API-to-Fabric lifecycle | PASS, fresh disposable kind/Calico; test 448.36 seconds |
| Console-to-API-to-Fabric | PASS, 25 real browser checks on 31 October; identical committed console source |
| Authorization and operation safety | PASS, invalid/expired tokens, missing deploy grant, foreign tenant/project, stale preview, idempotency, restart/adoption |
| Kubernetes isolation | PASS, foreign Secret/cluster-authority denials, enforced cross-namespace ingress/egress, positive connectivity control, admission limits and API-token restrictions |
| TLS and credential protection | PASS, live peer certificate checks; Secrets/public CRs/ConfigMaps/Helm histories/logs/source/evidence inspected; actual process-argument sampling required |
| Peer/PVC recovery | PASS, changed peer pod UID with preserved claims and advancing committed state; no disk-loss/backup-restore claim |
| Regressions | PASS: 283 Go cases, vet, 22 console cases/build/format, 41 PostgreSQL cases, restricted-role/TLS container qualification, 29 Python cases; 17 Go opt-in skips and one PostgreSQL skip recorded |
| Existing six-scenario atomic peer-TLS qualification | Preserved, not rerun; controller image unchanged; does not qualify all Fabric identity rotation |
| Generic customer configurations A/B/C | NOT TESTED at runtime; configurable contract validates intent only |
| Organization/MSP or Raft membership changes | NOT IMPLEMENTED; unsupported API actions rejected |
| General channel/configuration and anchor changes | NOT IMPLEMENTED |
| Actual chaincode package/code upgrades, private collections | NOT IMPLEMENTED |
| BFT execution | NOT IMPLEMENTED / NOT TESTED; compatible images existing locally do not qualify execution |
| Multi-server Kubernetes, disk-loss and backup restore | NOT TESTED; proven environment is Docker-hosted kind |
| Real Okta | BLOCKED by unresolved interactive enrollment/authentication; local-token qualification is not real Okta acceptance |
| Historical exposed-key remediation | BLOCKED; inventory, owner-approved revocation/rotation and recovery evidence required; retained network changes remain prohibited |
| AWS EKS / IRSA / VPC CNI / EBS | DEFERRED; Sprint 3 remains open |

## Remaining implementation order

1. Build one protected configuration-driven executor around the existing
   topology contract and prove distinct A/B/C configurations with real committed
   transactions. The existing genesis helper still enforces named two-org,
   three-orderer inputs. Removing those checks would not implement safe dynamic
   identity, genesis, membership or policy reconciliation. Use an isolated,
   versioned generator/worker and preserve this qualified profile.
2. Add durable configuration-update/signature workflows for organization/MSP,
   Raft and channel/anchor membership, with continuity and recovery acceptance.
3. Add real code/package upgrades and their dashboard flows, then version-gated
   BFT execution and independent runtime qualification.
4. Resolve external production gates: real Okta, historical key remediation,
   independent-host storage/recovery and deferred AWS acceptance.

The approval/retry implementation is in
[lifecycle.go](../internal/deployment/lifecycle.go), the guarded restart in
[lifecycle_restart.go](../internal/deployment/lifecycle_restart.go), and the real
restart/adoption acceptance in
[secure_api_e2e_test.go](../internal/httpapi/secure_api_e2e_test.go).
