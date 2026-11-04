# Fabric SMP by Sathonate

React + TypeScript interface for Sathonate's Hyperledger Fabric platform. This is an independent Git repository. The Go API is in E:/Sathonate/fabric-control-plane and communicates only through HTTP.

## Secure local Sprint 1 workflow

Sprint 1 secure browser-to-Fabric acceptance passes. See [the completion report](docs/sprint1-completion.md) for scope, evidence and production gates.

Use the verified backend at commit `aed4145402da9ea9b18c3adc64980e3903eaea88`. The secure workflow uses its prebuilt API and worker image; the console never imports backend code or invokes a lab issuer. Do not use the historical fixed-developer authentication or original controller for this workflow.

For an interactive operator-prepared local runtime, configure the backend's explicit `-mode local-development -deployment-mode secure-local`, trusted `-secure-local-config`, PostgreSQL and hashed `-local-token-grants` file as described in `E:/Sathonate/fabric-control-plane/docs/sprint1-secure-integration.md`. Generate the short-lived token interactively with the backend provisioning command, without output transcription. Never put tokens in environment files, logs or URLs.

Then start this console:

```powershell
$env:VITE_APP_ENV='local-development'
$env:VITE_LOCAL_AUTH_ENABLED='true'
npm run dev
```

Open http://127.0.0.1:5173, authenticate with the development token, select its authorized project, and use the existing six-step wizard. The profile fixes Fabric 2.5.15, LevelDB, two organizations/peers, three Raft orderers and smpchannel. The backend controls the cluster and namespace. Only the network's display name is editable in this profile.

Tokens stay in memory. Refresh requires sign-in again; the selected project's non-secret draft/operation identifiers restore its server-persisted progress. Sign-out clears authentication. Production builds and ambiguous/staging configurations reject local-token mode, and the fixed developer bypass is disabled. The existing Okta implementation remains for Sprint 2; Okta is not configured or qualified here.

For the complete self-cleaning qualification, with Docker Desktop, the existing `smp-postgres` fixture, qualified backend binaries/images, kubectl, kind, Python and Edge available:

```powershell
npm test
npm run format:check
npm run build
npm run test:e2e
```

The browser test uses a fresh disposable kind cluster and the real frontend and backend. It does not rebuild the backend. Ports 8080, 5173 and 5174 must be free; the fixture never stops existing listeners. Hashed server grants are temporary; client tokens travel only through an anonymous process pipe and the browser's in-memory sign-in. The fixture removes its cluster, credentials/configuration files, browser and app listeners. PostgreSQL operation history remains in the dedicated test database for audit. The retained network is checked read-only.

## PDF-based workflow

The entry point uses SmpApp.tsx and six screens: Cloud Account -> Kubernetes Configuration -> Network Design -> Review & Cost Estimate -> Deployment Progress -> Network Dashboard. Step 1 of the supplied PDF defines the light vertical step rail, white header, single bordered content panel, blue actions, provider cards, field layout and footer across all six screens. The former dark rail, horizontal steps and external summary column have been removed. See docs/pdf-design-system.md.

Open **Cloud readiness** for the missing AWS/EKS, Okta and HTTPS setup, a downloadable non-secret configuration template and read-only access checks. The cloud account and Okta application still need to be created; cloud execution stays disabled until the cloud adapter and production environment are qualified.

- Conditional AWS/Azure reference forms and a working Local / Existing Docker profile.
- New/existing cluster planning, node counts, CIDR checks and add-on selections.
- Organization, peer, Raft orderer, CA, channel and initial contract configuration.
- Grouped review, editable summaries, topology preview, backend validation and explicit cost assumptions.
- Desired Fabric designs saved through the Go API with revision checks; non-secret provider/cluster-planning fields restore within the current browser tab.
- Real local CA enrollment, containers, channel creation and Go contract verification.
- Actual operation stages, persisted events/retries and a live Fabric/CA dashboard with logs and transaction checks.
- Mobile layouts, accessible controls and optional Okta sign-in without password/key collection.

Cloud authorization, provisioning and pricing remain unavailable. The local executable profile is fixed and distinct from cloud designs. No mock success, cloud prices, transaction samples or resource metrics are displayed.

## Checks and legacy scenarios

The secure qualification entry point is `npm run test:e2e`; evidence and screenshots are written to ignored `.logs/`. It includes invalid/expired tokens, unauthorized project selection, real worker failure/retry, refresh, network interruption, live Fabric transactions, cross-scope denial, missing permissions, mobile layout, production-build rejection and runtime secret/RBAC inspections.

The historical browser scenarios remain available under `npm run test:e2e:legacy` for reference. They assume the earlier fixed-developer/Docker/cloud-planning setup and are not qualification for the secure bearer profile. They were not represented as passing in this sprint. Port them to an authorized environment before future use; do not restore an authentication bypass to run them.

Build output is dist/. Production hosting needs HTTPS, SPA history fallback and a same-origin /api reverse proxy. Real Okta/EKS qualification and production packaging remain outstanding.

See docs/pdf-design-system.md for the design decisions and capability boundaries.
