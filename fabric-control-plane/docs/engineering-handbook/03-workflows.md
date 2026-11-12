# 03 · Workflow sequences

[Handbook](README.md) · [API](05-api.md) · [Recovery](09-operations.md)

## Authentication and onboarding

**Why.** Establish verified identity before selecting a server-owned project and before any tenant-wide account administration.

```mermaid
sequenceDiagram
  participant B as Browser
  participant I as Okta
  participant A as API
  participant D as PostgreSQL
  B->>A: GET auth config
  B->>I: Authorization code request with PKCE
  I-->>B: MFA and callback
  B->>A: Access token and GET projects
  A->>I: Verify against configured JWKS
  A->>D: Lookup exact issuer, subject and environment
  D-->>A: Server-owned project membership
  A-->>B: Authorized project
  B->>A: Read account or request explicit write scope
  A->>D: Transactional customer action and audit
```

`AuthGate` initializes Okta and uses a memory token manager. `Authenticator.Middleware` validates the access token, checks assurance and route permission, and derives the project scope from the approved grant. The account owner must separately match issuer and subject. The admin CLI bootstraps an existing owner grant. A viewer may accept only their own pending invitation, without gaining a Fabric grant. Missing grants stop at project selection. Failed writes can be retried with the same idempotency key and body. [Identity](../../internal/identity/okta.go) · [Customer actions](../../internal/customer/actions.go) · [API fixture](../../internal/httpapi/customer_test.go) · [Browser fixture](../../../fabric-console/e2e/customer-pilot.spec.ts).

## Provision, expand and verify transactions

**Why.** Keep design intent, admission, irreversible Fabric configuration and committed-state evidence separate.

```mermaid
sequenceDiagram
  participant U as Console
  participant A as Go API
  participant J as Operation journal
  participant W as Qualification worker
  participant F as Fabric
  U->>A: Save topology and get revision
  U->>A: Submit deployment with revision and idempotency key
  A->>J: Persist operation and target binding
  A-->>U: Accepted operation identifier
  J->>W: Dispatch through enabled runner
  W->>F: Enroll, start nodes, create and join channels
  W->>F: Install, approve and commit chaincode
  W->>F: Endorse, submit and verify committed state
  W-->>J: Verification and observed result
  U->>A: Preview expansion against parent and revision
  A-->>U: Reviewed change digest
  U->>A: Submit reviewed expansion
  A->>J: Create linked operation
```

1. The domain validator checks identities, references, ordering capability, and desired topology. Target and profile guard dispatch. Customer Kubernetes currently returns a preview only.
2. The manager persists before dispatch. The worker enrolls identities and prepares protected material, deploys orderers and peers, joins channels and commits chaincode.
3. An operation needs actual transaction and peer readback evidence. For supported expansion, `PreviewDynamic` and `ApplyDynamic` bind the parent and digest, then reconcile approved additions and verify again.
4. Stale revision, wrong target, incomplete trust, disagreement or an interrupted worker blocks progress. Inspect the existing operation and receipts. Retry the same approved target through the manager. Once a channel configuration changes, use reviewed forward recovery where rollback is unsafe.

[Manager](../../internal/deployment/manager.go) · [Dynamic changes](../../internal/deployment/dynamic.go) · [Worker](../../internal/kubernetesfabric/worker.go) · [A/B/C evidence](../sprint4-dynamic-acceptance.md).

## Monitoring and recovery

```mermaid
sequenceDiagram
  participant U as Console
  participant A as API
  participant M as Manager
  participant K as Workloads
  participant P as Prometheus
  U->>A: Read operation, health and diagnostics
  A->>M: Query scoped workload observations
  M->>K: Inspect approved resources
  K-->>U: Observations via API
  U->>A: Preview and approve node recovery
  A->>M: Check identity, binding and current state
  M->>K: Reconcile approved node
  P->>A: Scrape separate operations listener
```

The UI distinguishes recorded history and live observations. Verify workload/PVC identity before and after recovery, then confirm Fabric ledger continuity. A stale preview or unhealthy quorum stops recovery. Metrics are process-wide and use bounded labels. [Workloads](../../internal/httpapi/workloads.go) · [Recovery](../../internal/deployment/node_recovery.go) · [Telemetry tests](../../internal/telemetry/http_test.go) · [Local evidence](../sprint5-acceptance.md).

## Billing, backup, restore and upgrade

```mermaid
sequenceDiagram
  participant P as Fixture payment verifier
  participant S as Customer service
  participant D as Scoped PostgreSQL
  participant M as Meter job
  P->>S: Signed event and provider binding
  S->>D: Deduplicate, update state, append audit
  M->>D: Read confirmed scoped execution receipts
  M->>S: Acknowledge each qualifying operation
  S->>D: Record usage once
```

Billing events verify signature and customer mapping before changing subscription or invoice state. A replay keeps the original effect. The meter may reread a window after interruption. [Payment](../../internal/customer/payment.go) · [Meter test](../../internal/customer/meter_test.go) · [Commercial workflow](10-commercial.md).

```mermaid
sequenceDiagram
  participant O as Authorized operator
  participant H as Cold backup helper
  participant K as Bound Fabric workloads
  participant A as Encrypted artifact
  O->>H: Review capture scope and executor fence
  H->>K: Quiesce writers and capture bound state
  H->>A: Seal and record digest
  H->>K: Resume original workloads
  O->>H: Prepare restore using exact artifact
  H->>K: Check UIDs, divergence and stopped writers
  A-->>H: Authenticate and restore eligible state
  H->>K: Resume and independently verify ledger
```

A failed scheduled slot needs review before later slots proceed. Restore verifies original ownership and rejects divergence. Replacement targets require distinct implementation and acceptance. [Cold backup](../../internal/deployment/cold_backup.go) · [Schedule](../../internal/maintenance/schedule.go) · [Sprint 7 evidence](../sprint7-acceptance.md).

```mermaid
sequenceDiagram
  participant O as Operator
  participant R as Rollout guard
  participant D as Trusted adapter contract
  O->>R: Preview exact image transition
  R->>D: Require fresh ledger, quorum and storage evidence
  O->>R: Submit reviewed digest
  R->>D: Reobserve under mutation fence
  R->>D: Build UID and revision checked image patch
  D-->>O: Controlled result only
```

Production mutation wiring is blocked. Use the approved release edge and CA enrollment pause when applicable. Never treat a mocked patch as a workload rollout. [Upgrade guard](../../internal/rollout/upgrade.go) · [Mocked patch test](../../internal/rollout/patch_test.go).
