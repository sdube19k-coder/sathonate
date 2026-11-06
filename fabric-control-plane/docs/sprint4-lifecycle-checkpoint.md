# Sprint 4 lifecycle implementation checkpoint

## 6 November: mandatory local dynamic MVP PASS

[Final report](sprint4-dynamic-acceptance.md) and [machine evidence](evidence/sprint4-dynamic-final/summary.json) supersede earlier partial checkpoints. Backend `e61e216`, console `c4926fb`. The fixed baseline and final-source A/B/C passed; live dashboard passed nine checks, runtime credential/RBAC/TLS inspections and cleanup. Final PostgreSQL: 42 PASS / one explicit TLS-fixture skip. Go 348 PASS / 18 opt-in skips; helper 8 PASS; console 25 PASS/build/format; Python 19 PASS. Opt-in execution is recorded separately, never silently counted as ordinary Go unit PASS.

Preserve successful runs without unnecessary repetition:

- Baseline `.logs/dynamic-baseline-20261105-223436` (535.46 s API).
- A `.logs/dynamic-a-20261105-224709` (642.81 s API; three independent expansions).
- B `.logs/dynamic-b-20261105-230055` (519.88 s API).
- C `.logs/dynamic-c-20261106-103403` (470.61 s API).
- Browser `fabric-console/.logs/dynamic-browser-20261106-104804` (nine checks).

All four API runs used immutable worker `sathonate/fabric-worker@sha256:b38d1af7b5e38a50806daad7a2c9b73259f6226527f26203b8196b07380d0f78` (image ID `sha256:25e250821fc530886d76f1d0c0331fe467b203f8f282fde0a5e6ea0fc090242d`). All 184 Go files in its saved source receipt match the committed backend exactly. Controller pin remains unchanged. Each successful disposable cluster was removed. The interrupted overnight C attempt was NOT PASS; its provider-ID-verified cleanup is saved separately. Only retained `smp-qualification` remains; read-only readiness is 8/8.

The reproducible added-peer failure was channel-history delivery lag: query at genesis before existing definition blocks arrived. `channel_sync.go` now gates verification on authenticated all-peer height/hash convergence. Forks and trust/query errors fail closed. See the filtered public root-cause timeline and final baseline transactions; do not weaken policy or indiscriminately retry endorsement errors.

Runtime source and evidence are committed in logical increments. Final credential/history scan is required before pushing the evidence increment. Do not overwrite earlier failed/partial receipts or represent them as successful runs.

Sprint 3 stays OPEN: LOCAL QUALIFICATION PASSED / AWS EKS QUALIFICATION PENDING. AWS/production remain disabled. Real Okta MFA and historical-key remediation remain external production blockers. Kind does not qualify physical multi-server deployment or disaster recovery. BFT, removal/root/storage/placement replacement/private collections and dynamic API deletion remain unqualified.

The user authorized Sprint 5 after local MVP completion and selected the existing local operations/recovery backlog. Begin a read-only, strictly scoped workload troubleshooting increment; do not bypass ownership/authentication or mutate the retained network. Track its new source/tests separately from this verified Sprint 4 snapshot.

## Historical checkpoints

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
