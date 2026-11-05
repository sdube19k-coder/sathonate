# Sprint 4 lifecycle implementation checkpoint

## 5 November: A/B/C passed; completion blocked

Active workspace: `E:/Sathonate/sathonate`, private repository `sdube19k-coder/sathonate`. Preserve all uncommitted execution/dashboard work. The original repositories, qualified controller and retained network are unchanged. AWS and production remain disabled.

Fresh sequential disposable acceptance passed for all three customer-defined configurations:

- A: two initial organizations, two peers and three Raft orderers; independently approved peer addition, organization/membership/anchor addition, then Raft expansion to five, new channel and executable 1.0 to 2.0 upgrade. Four peers converged at height 18 on shipments and height 6 on the added channel.
- B: three organizations, four peers, three orderers distributed across organizations and two channels. All four peers reached height 7 on each channel.
- C: two organizations, three peers, five orderers and split signing/TLS authorities. All three peers reached height 6.

Every case passed authenticated API orchestration, actor/project isolation, API restart/adoption, worker failure/retry, persistent claims after peer restart, live certificate checks, credential/private-key inspection and effective Calico network isolation. A's three change operations preserved existing workload UIDs. Each A/B/C disposable cluster was removed. Retained readiness was checked read-only at 8/8 in each successful run. These observations are dated 5 November; they do not establish current readiness while Docker is unavailable.

The subsequent fixed-topology lifecycle regression FAILED on an endorsement error from a `chaincode-query` against `org1-peer-2`. The redacted error does not establish its precise root cause. Its final JSON receipt is NUL-corrupted, so teardown is unverified. Preserve the failed attempt; do not classify it as an environment-only failure or PASS. Docker now reports the Linux engine pipe missing. Runtime tests are stopped until Docker is restored; do not repeatedly probe the inaccessible engine.

Fresh local verification: Go 344 test events PASS / 18 opt-in skips, vet PASS; isolated config helper 8 test events PASS and vet PASS; console 25 tests, build and format PASS. Final PostgreSQL rerun, fixed-topology regression, live dashboard acceptance and final review/commit of the engine remain open. Runtime changes stay uncommitted until those gates pass. Deterministic public CONFIG update serialization is an independently tested helper increment.

Evidence and exact transaction IDs: `docs/evidence/sprint4-dynamic/summary.json`, matching A/B/C API and security receipts, and `baseline-failure.log`. Current source review hashes are explicitly separate from execution-image provenance. Older failed/debug attempts remain under `.logs/dynamic-*`; never substitute them for complete acceptance.

Resume in normal PowerShell with Docker's Linux engine running, from the backend:

```powershell
$env:GOCACHE='E:/Sathonate/.cache/go-build'
$env:GOPATH='E:/Sathonate/.cache/go'
$env:GOPROXY='off'
$env:SMP_CACHE_ROOT='E:/Sathonate/.cache'
python tools/sprint2/postgres.py --output .logs/sprint4-dynamic-resume-postgres.jsonl
./tools/sprint4/qualify-dynamic.ps1 -Topology baseline
```

First inspect disposable leftovers using `E:/Sathonate/tools/kind.exe get clusters`; clean only the failed run's recorded `smp-sprint1-*` cluster after context/provider-ID validation. Never delete `smp-qualification`. Diagnose the baseline error without exporting private diagnostics or weakening endorsement/MSP/TLS. Only after its PASS, refresh the dynamic image/API build receipt and execute `fabric-console/tools/qualify-ui.py` with `SMP_UI_DYNAMIC=1`, an exact `SMP_UI_API_SHA256`, shared cache/kind/backend paths and a unique evidence label. Browser qualification is NOT TESTED; its script is not acceptance evidence.

Sprint 4 remains PARTIAL. Sprint 3 remains OPEN: LOCAL QUALIFICATION PASSED / AWS EKS QUALIFICATION PENDING. Real Okta MFA, historical-key remediation owner approvals, EKS/IRSA/CNI/EBS, genuine multi-server testing and production readiness remain open. BFT, removals, private collections and replacement of roots/storage/placement are not qualified and must remain rejected. Public runtime configuration uses reviewed server-approved executable artifacts; arbitrary customer executable images are not permitted.

## 4 November resume

The 31 October final browser run completed PASS: all 25 browser checks, the
four approved lifecycle changes, runtime credential/private-key inspection,
foreign-resource/RBAC checks, live peer certificates and disposable cleanup.
The original timestamps and complete redacted report are preserved in
`docs/evidence/sprint4-lifecycle-browser.json`. All three peers reached equal
heights 6, 10, 11 and 12 after addition, definition upgrade, restart and the final
verification transaction respectively. Final transaction:
`94cf22e43f2243dca764987e7c63760677119a6ff248d93192452507c9546f66`.

On 4 November the separate API-restart/Calico lifecycle acceptance PASSED
(448.36 seconds), followed by all 14 runtime security/isolation checks and
disposable cleanup. Each of the four lifecycle operations survived application
restart/adoption. Three-peer heights advanced through 7, 11, 12 and 13, with
equal block hashes and new committed/read-back transactions. The final ID is
`316da2c9b794d742b1bd2fb159e1b6a1647ba0d8715225903970d50e9a593fcd`.

Retained readiness was checked again read-only: 8/8 deployments ready and seven
PVCs Bound. Only the stopped PostgreSQL test fixture was started. Fresh Go tests
(283 PASS, 17 opt-in skips), vet, 22 console tests/build/format, 41 PostgreSQL
regressions (one skip), PostgreSQL TLS/restricted-role and 29 Python safety tests
passed. Backend source is committed as `fbdd205`; console source as `e023c17`.
Source hashes, full transaction/ledger evidence, image identities, security
matrix and cleanup are in `docs/evidence/sprint4-lifecycle-verification.json`.

The console now recovers bounded temporary failures during initial read-only
workspace loading. Authentication/authorization errors stay terminal. The live
browser run deliberately interrupted the first metadata request and verified
recovery, then passed missing-permission and cross-project/tenant denials.

Sprint 4 is PARTIAL. The generic topology package remains design validation,
not a provisioning engine. Organization/Raft membership, general channel and
anchor changes, actual code upgrades and BFT execution remain unfinished.
Definition sequence 2 retains the qualified package/image. Sprint 3 stays OPEN:
LOCAL QUALIFICATION PASSED / AWS EKS QUALIFICATION PENDING. AWS and production
remain disabled. Earlier failed attempts below are historical, not the latest
browser result.

## Historical 31 October investigation — third-peer failure isolated

Sprint 4 remains PARTIAL. The next fresh acceptance run is in progress; no new
lifecycle PASS is asserted here. AWS remains prohibited.

The precise third-peer failure was isolated on a guarded disposable fixture.
Its package was installed, but the peer remained at ledger height 1. Redacted
peer diagnostics reported empty gossip membership and failure to fetch the
Org1 implicit private `_lifecycle` collection. The pinned peer renderer defaults
to an empty gossip bootstrap. The added peer therefore could not fetch the
existing organization's private approval data and advance through block 1.
This was not evidence that the endorsement policy needed weakening.

A diagnostic-only change to that disposable peer's bootstrap, pointing to the
existing Org1 peer, let it reach height 6 and read the newly committed key. That
manual diagnostic is NOT an acceptance PASS. The source fix in
`internal/kubernetesfabric/gossip.go` derives sorted bootstrap endpoints only
from other managed peers in the same organization. `secureNode` applies it when
creating a new peer; it does not patch existing peers. Its regression excludes
self, orderers and peers from other organizations.

Canonical orderer block-zero fetching and bounded retry of the sample
contract's explicit absent-key response were added during investigation. Neither
alone resolved the failure. Genesis validation checks block number, channel
header and data hash; trust and authorization errors remain terminal. The fresh
browser and separate API/restart qualifications must still pass before the
uncommitted lifecycle implementation can be committed.

An interrupted earlier test process left only a disposable kind fixture behind
after Docker restarted. The provider-ID guard was verified and that fixture was
removed. The PostgreSQL test container was restarted; the retained Fabric
network was inspected read-only and remains 8/8 ready.

Review also found that the legacy endorsement expression used the first two
peer positions. With the added Org1 peer, those positions can both belong to
Org1. That disposable attempt was stopped before any upgrade execution. The
replacement `endorsement.go` builds the AND expression from distinct MSPs and
regresses duplicate peers, reordered peers, missing organizations and expression
injection. Committed definitions are still independently decoded and required
to retain the exact Org1 AND Org2 policy. A rebuilt worker and API are under
fresh acceptance; the stopped attempt is not counted as a test PASS.

2026-10-30. UNCOMMITTED implementation; Sprint 4 PARTIAL. AWS remains prohibited.

Implemented bounded local-only lifecycle previews and digest approval, durable prepared submissions/adoption/retry, third peer org1-peer-2, chaincode definition sequence 1 -> 2 with both organization approvals/readiness/AND endorsement verification, UID-bound peer restart and new verification transactions. Console controls and observed topology labeling are implemented. Original fixed-profile path remains separate; original controller and retained network untouched.

Two runtime failures are preserved in .logs/sprint4-lifecycle-attempt1 and attempt2. First: argument sampling evidence missing after successful transaction; live /proc sampling now retries empty exec observations, without reducing required evidence. Second: unsupported singular Kubernetes resource name in new preflight, corrected to fabricpeers before mutation. Both clusters removed. PostgreSQL and local unit/build suites pass. Current browser qualification is running; no new lifecycle acceptance PASS or production readiness is claimed until it finishes.

Raft/organization membership, general channel configuration/anchor updates, arbitrary chaincode code/collection deployment remain unimplemented and disabled. Current chaincode upgrade changes the definition sequence/version while preserving the approved package/image; it is not arbitrary code upgrade support. Live Okta, historical key remediation, EKS/IRSA/CNI/EBS and multi-server recovery remain open.

Browser resume: build bin/fabric-api-sprint4-lifecycle.exe, set SMP_UI_LIFECYCLE=1 and SMP_UI_API_SHA256 to its SHA256, then python tools/qualify-ui.py from console with the approved kubectl on PATH. Worker image receipt: backend .logs/sprint4-lifecycle-worker-build.json. Browser logs: console .logs/sprint4-lifecycle-browser-runtime.log and browser.log. Do not commit unverified lifecycle changes. Preserve pre-existing AWS budget drafts outside this change.

## Historical enrollment failure — superseded by the qualified results above

Fresh browser/API deployment succeeded on the original two-peer profile, including its initial committed transaction, refresh and worker retry. Approved peer-addition operation `op_95e1f7fdf938bcfa536c5619862cc5b6` failed during protected identity enrollment for org1-peer-2, before its FabricPeer CR was created. Only the new enrollment-password Secret name was observed; no credential contents were printed. Existing two peers, two CAs and three orderers remained ready. The precise CA registration/enrollment failure category is still unresolved; do not weaken trust or registrar permissions to bypass it.

Browser qualification was stopped after the known terminal worker failure to avoid waiting for an already-failed success assertion. Its Python finally block removed the disposable cluster and stopped test API/Vite processes. No new lifecycle acceptance PASS is claimed. Upgrade/restart/new-transaction-on-expanded-topology cases were not reached. All lifecycle implementation changes remain UNCOMMITTED. Prior qualified commits remain intact.

Further work: isolate registration versus enrollment versus certificate authorization failure using fixed redacted categories; add a regression, rebuild worker and repeat actual peer-addition acceptance, then run upgrade/restart and final-source API/browser/security suites. Preserve failed attempts and do not commit unverified code. Sprint 4 remains PARTIAL. Sprint 3 remains OPEN with AWS qualification deferred.

## Completion-mandate continuation

Verified commits: `f8eea3a` fixes restored registrar signing-certificate filename to Fabric CA client's required signcerts/cert.pem, with dependency-contract regression. Live browser run progressed through enrollment and channel join of org1-peer-2 after this fix; full peer-addition acceptance subsequently failed in a Bevel SDK lifecycle query. `e7ee76e` adds bounded configurable topology intent validation only; it deliberately reports executionEnabled=false and qualified=false. It is NOT configurable network provisioning.

The secure worker now uses direct, TLS-verified official peer CLI calls for installed packages, installation, committed-definition queries and organization approvals, rather than relying on SDK target discovery for expanded topology. This change is still uncommitted pending fresh runtime qualification. A retry fence rechecks supersession inside the manager lock. Existing working fixed-profile commits remain intact.

Precise remaining delivery gaps: protected engine/genesis builder still assumes two organizations, three Raft orderers and one channel; dynamic organization/orderer config-update signing, membership checkpoints and continuity acceptance are not implemented. BFT-capable Fabric 3.1.5 images exist locally, but namespace-scoped genesis/configuration/worker adapters and real BFT acceptance do not. Arbitrary chaincode implementation/package upgrades and private collections are not implemented. No missing-image claim is made. The general topology contract does not waive any of these execution gaps. Sprint 4 remains PARTIAL.

## Historical endorsement failure — superseded by the qualified results above

The restored registrar fix passed its live boundary: org1-peer-2 was enrolled, created and joined. Subsequent package/lifecycle work still failed. Replacing SDK package handling and then SDK lifecycle queries with direct official peer commands advanced execution, but the latest attempt failed with fixed category `endorsement` after third-peer package checks. No synchronized third-peer transaction, definition upgrade, or approved restart PASS is claimed. Disposable fixture teardown completed; preserve latest console .logs/browser.log and .logs/ui-qualification.json. Initial fresh baseline transaction in this attempt: c395140aa4c4a61f3fb678a96f59f69187d4c6d72eefea6a534fe613dd7c43eb, both original peers height 5 with equal hashes. This is not expanded-topology acceptance.

Next debugging boundary: identify which direct peer lifecycle command (package install versus committed-definition query) returned endorsement failure using a constant operation-category annotation; inspect no raw credential-bearing diagnostics. Fix and regress that boundary, then rerun synchronized third-peer transactions and remaining lifecycle/browser/security tests. All broader lifecycle execution changes remain uncommitted and unqualified. Verified commits are f8eea3a (registrar restoration fix) and e7ee76e (design validation only). Do not label Sprint 4 complete.
