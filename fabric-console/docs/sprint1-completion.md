# Sprint 1 completion — 24 October 2026

Overall status: **PASS for the supported local-development profile**. Sprint 2 has not started. Production and AWS remain disabled and unqualified.

This closes the separate-console UI blocker recorded by backend commit `aed4145402da9ea9b18c3adc64980e3903eaea88`. The backend source, native API binary, candidate controller and worker image were preserved; no backend rebuild or source change was needed. The console repository had no existing commits, so its reviewed initial commit includes the existing UI foundation plus this integration. Historical cloud/Docker browser scenarios are retained, not claimed as current qualification.

## Implemented flow and source

- [Authentication](../src/AuthGate.tsx) preserves Okta and adds explicit development gates. Fixed developer authentication cannot bypass sign-in.
- [Local sign-in](../src/LocalSignIn.tsx) holds the token in memory, discovers server-granted projects and validates selection. Invalid/expired authentication clears the workspace.
- [Project scope](../src/workspace.tsx) separates non-secret browser drafts by the server-returned tenant/project tuple. The API enforces authorization; the console sends no tenant/project override headers.
- [API client](../src/api.ts) supplies bearer headers, stable idempotency keys and meaningful authentication/authorization/connection errors.
- [Profile model](../src/smp-model.ts) and [existing wizard](../src/SmpApp.tsx) restrict the secure profile to the backend-qualified topology, validate it with the server and track durable operations. Unchanged designs preserve their revision, and a submit guard prevents repeated clicks from creating conflicting requests.
- [Existing progress/dashboard](../src/SmpRuntime.tsx) display actual persisted stages, errors, retry linkage, CA/peer/orderer/channel/chaincode health and transaction evidence. Recorded verification is explicitly labelled; its control does not redeploy the network or claim to submit a fresh transaction.
- [Browser acceptance](../tools/browser-acceptance.mjs) operates the actual application; [fixture/inspector](../tools/qualify-ui.py) creates only an isolated test environment, injects one owned worker-pod failure, inspects material in memory and removes it.

The supported profile is `smp-secure-local-v1`: Fabric 2.5.15, Fabric CA 1.5.15, LevelDB, Org1MSP/Org2MSP, one peer per organization, three Raft orderers and smpchannel. The existing design system and six-step wizard are preserved. Cluster/context/namespace choices, extra organizations, alternate consensus/state databases and cloud providers are not offered by this profile.

## Authentication and state semantics

Local token mode requires Vite development mode, `VITE_APP_ENV=local-development` and `VITE_LOCAL_AUTH_ENABLED=true`. Staging/production/ambiguous values fail closed. Production-build rejection was also exercised in a real browser against the local-token API. Okta code is preserved but not configured or qualified.

The browser stores no bearer token in localStorage, sessionStorage or URLs. A refresh requires authentication again. Project-specific non-secret draft and operation identifiers then restore status from PostgreSQL through authorized APIs. If the initial deployment acknowledgement is lost, unchanged revision/idempotency intent and server operation discovery prevent conflicting submissions. Permission failures remain server-authoritative and visible; selecting an option does not grant permissions.

The dashboard's readback rows reflect the backend's completed deployment verification: it queried the committed sample value on each peer before recording its verified health. The API exposes the transaction ID, peer identities, ledger heights and matching block hashes, rather than the application value itself. The UI labels this recorded result and its verification time; it does not manufacture fresh reads from workload readiness.

## Executed acceptance matrix

| Check | Result |
|---|---|
| Existing nine unit regressions plus seven secure/transport/render tests | PASS, 16 total |
| TypeScript and production build | PASS |
| Actual Edge browser: invalid and expired tokens | PASS, real server 401 responses |
| Unauthorized project selection | PASS |
| Explicit development gates / production build / fixed developer bypass | PASS; legacy-config countercheck is an auth-only negative fixture |
| Secure profile, server validation and UI deployment submission | PASS |
| Repeated-click/idempotency checks | PASS |
| Refresh while running and after completion | PASS; same persisted operation restored after reauthentication |
| Network interruption | PASS; transport error displayed, polling recovers |
| Owned worker termination, visible failure and authorized UI retry | PASS |
| Two CAs, two peers, three orderers, channel and chaincode | PASS |
| Committed transaction and both peer readbacks/ledger convergence | PASS |
| Desktop and 390px mobile dashboard | PASS, screenshots reviewed |
| Cross-tenant/project reads and retry denial | PASS, real backend requests from the browser |
| Missing deployment permission | PASS, real server 403 displayed in the UI |
| Browser storage/URLs | PASS, no bearer credentials |
| Public CRs/ConfigMaps/pod definitions, all Helm history and runtime logs | PASS; 12 Helm history records and 10 log streams |
| Actual credentials compared in memory against source, build and evidence | PASS; 91 private values, 88 files at inspection |
| Live certificates, CA/DNS trust on both peers | PASS |
| Controller/issuer foreign Secret and cluster privilege denial | PASS; foreign canary preserved |
| Cleanup and retained network | PASS; disposable cluster removed; retained deployments 8/8 ready, read-only |
| Backend regression baseline | Existing verified PASS preserved; no Go source changed and no backend rebuild/regression rerun was needed |

All 18 browser checks and seven runtime inspection checks passed in the final clean run from the recorded final application/fixture inputs. A prior complete run also passed; the final repeat verifies the documented npm entry point after a minor label encoding/wording correction and an added render test. The first browser attempt exposed an inaccessible project-selector label; a second run completed Fabric but failed mobile overflow. Both failed attempts are retained separately and are not acceptance evidence.

## Actual final identifiers

- Disposable cluster: `smp-sprint1-40ec3c47` (removed).
- Initial operation: `op_302f22a9336e61076c42d98ec51e1fde`.
- Successful retry: `op_b9c8420d9c401128b93e4a39c9101773`.
- Committed transaction: `a228274c94117b64750c7831ff0d4e5585147dd3a9d188eafe11421126f2d87e`.
- Both peers and all three orderers reached height 5; both peer block hashes match.
- Candidate: `sathonate/controller-candidate@sha256:427b352b20576258efdb59c9fc266ec01fe7b7f1419ce1e5d7ead488b10cba6c`.
- Worker: `sathonate/fabric-worker@sha256:500a97af7e8dc99585306c6ce1893b66daf4b74e3a1a6490dff4bcfb31804fcf`.

Machine-readable results are `.logs/browser-result.json` and `.logs/ui-qualification.json`. The final manifest records source/build/log hashes and reviewed commits. Screenshots are `.logs/dashboard.png` and `.logs/mobile-dashboard.png`. Tokens, private keys and raw Helm/Secret contents are not exported. The fixture generates new client tokens only in memory and writes hashes to its temporary server grant file; browser tracing is disabled.

## Reproduction and remaining gates

Follow the README commands. `npm run test:e2e` requires the unchanged backend baseline and its verified local binaries/images, the dedicated PostgreSQL fixture, Docker Desktop, kind, kubectl, Python and Microsoft Edge. It checks provenance before launching the real API and console. It does not use an API-only deployment test or a Python issuer as a substitute for browser acceptance.

Sprint 2 may now begin, but it has not been started here. Mandatory production gates remain: Okta tenant/project membership, restricted database roles and production broker/issuer authorization; admission/CNI containment; separately authorized handling of historical retained-controller key exposure; broader CA/signing/auxiliary/Raft rotation and backup/restore qualification; and actual AWS qualification only after explicit authorization. Peer TLS qualification does not establish all Fabric identity rotation. No retained network, original controller, AWS resource or production deployment was modified.
