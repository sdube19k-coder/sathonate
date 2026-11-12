# 02 · Codebase map

[Handbook](README.md) · [Backend catalog](catalog-backend.md) · [Console catalog](catalog-console.md) · [API catalog](catalog-api.md)

The pinned catalogs enumerate both tracked trees, including the source, tests, scripts, manifests, examples and evidence. The catalogs link source declarations and direct imports where identified. This page describes how the packages fit together.

| Backend package | Responsibility and boundary |
|---|---|
| [acceptance](../../internal/acceptance) | Harness with explicit evidence and execution boundaries. Acceptance does not infer success from a plan. |
| [audit](../../internal/audit) | Scoped attempt/outcome event capture, redaction and PostgreSQL append/read boundaries. |
| [certificateprototype](../../internal/certificateprototype) | Candidate certificate/material primitives and proof fixtures, separate from production qualification. |
| [cloud](../../internal/cloud) | Target registry, intent compilation, preflight and readiness. Cloud provider preparation does not enable provisioning. |
| [customer](../../internal/customer) | Account and subscription state, onboarding commands, receipt-backed metering, payment/hosted interfaces and customer target preview. |
| [deployment](../../internal/deployment) | Runner selection, approval-bound orchestration, observed health, cleanup, recovery, cold artifacts, local and EKS qualifications. |
| [domain](../../internal/domain) | Network specification, revision-compatible defaults, issues and validation contracts. |
| [drafts](../../internal/drafts) | Local repository and draft representations for desired state, review and revision tracking. |
| [engine](../../internal/engine) | Engine contract definitions. |
| [httpapi](../../internal/httpapi) | Route registration, request/response boundaries and authentication-scoped repository/runner calls. |
| [identity](../../internal/identity) | Okta verification, grant evaluation, principal scope and explicit local bearer auth. |
| [kubernetesfabric](../../internal/kubernetesfabric) | Fabric worker, Kubernetes-side protection, material, channel and lifecycle reconciliation. |
| [localfabric](../../internal/localfabric) | Separate local Fabric runtime adapter and health representations. |
| [maintenance](../../internal/maintenance) | Public certificate health, sealed artifacts, guarded rotation records, backup schedule and PostgreSQL recovery helpers. |
| [operations](../../internal/operations) | Stage journal, idempotency, cancellation and recovery with local or PostgreSQL persistence. |
| [persistence](../../internal/persistence) | PostgreSQL connection, migrations, tenant-scoped network repository and membership authority. |
| [planner](../../internal/planner) | Derives plan stages and user-visible estimates from intent. |
| [readiness](../../internal/readiness) | Release capability/preflight status reporting. |
| [redact](../../internal/redact) | Secret-safe diagnostic transformations. |
| [rollout](../../internal/rollout) | Upgrade compatibility and quorum checks and image-only patch construction. Production driver remains absent. |
| [telemetry](../../internal/telemetry) | Bounded HTTP histograms, entitlement counters and readiness on a separate listener. |
| [topology](../../internal/topology) | Dynamic placement/storage/policy contract and customer storage checks. |

## Entry points and test surfaces

- [API](../../cmd/fabric-api), [console server](../../cmd/fabric-console-server) and [worker](../../cmd/fabric-worker) are the main runtime binaries. [fabricctl](../../cmd/fabricctl) and [local-chaincode](../../cmd/local-chaincode) support operator and fixture workflows.
- [Customer admin](../../cmd/fabric-customer-admin) and [meter](../../cmd/fabric-customer-meter) are scoped operational commands. [Membership](../../cmd/fabric-membership) and [local token](../../cmd/fabric-local-token) administer separate identity paths.
- [Cold backup](../../cmd/fabric-cold-backup), [PostgreSQL backup](../../cmd/fabric-postgres-backup), and [maintenance](../../cmd/fabric-maintenance) expose distinct artifact and lifecycle commands.
- [Cloud prepare](../../cmd/fabric-cloud-prepare), [EKS](../../cmd/fabric-eks), [acceptance](../../cmd/fabric-acceptance) and [qualification export](../../cmd/fabric-qualification-export) are preparation, qualification or evidence tools. Treat their execution scopes independently.
- [Helm](../../deploy/charts/sathonate), [images](../../deploy/images), [examples](../../examples) and [tools](../../tools) cover packaging, fixture inputs, protected controller work, and qualification runners. The pinned upstream Bevel dependency is a separate source boundary. [Provenance](../../../SOURCE_PROVENANCE.json).

## Console module groups

| Group | Responsibility |
|---|---|
| Entry and identity | `main`, `App`, `AuthGate`, `ProjectGate`, `LocalSignIn`, `workspace` set application shell and authorized project context. |
| Transport and models | `api`, `types`, `smp-model`, `network-form`, `custom-configuration`, `visual-network`, `read-recovery` define typed IO and edit/retry behavior. |
| Design and navigation | `SmpApp`, `SmpScreens`, `Wizard`, `OrderingStep`, `CustomNetworks`, `NetworkVisualBuilder` represent configuration and workspace interaction. |
| Runtime and operations | `SmpRuntime`, `RuntimeDashboard`, `OperationProgress`, `OperationDiagnostics`, `NetworkDetail`, `WorkloadStatus`, `LifecycleManagement`, `NodeRecovery`, `RetainedDeletion`, `LocalRuntime` expose runtime and maintenance workflows. |
| Commercial and launch | `CustomerAccount`, `CloudReadiness`, `LaunchReadiness` expose pilot account and launch constraints. |
| Presentation | Shared components, dialog handling, theme/style files and assets keep the shell consistent. Exact modules and tests are in the [console catalog](catalog-console.md). |

## Core backend component walkthrough

| Component | Why it exists | Main handoff and failure behavior |
|---|---|---|
| API composition | Keeps profile selection, identity, persistence and execution enablement in one explicit startup boundary. | `cmd/fabric-api` configures verified PostgreSQL and mounts the authorized handler. Production denies active execution. Invalid role or trust stops startup. |
| Design repository | Supports concurrent edits and review without creating resources. | `Database.ForTenant` constructs the scoped repository. Saves use a revision, history preserves desired versions, and retirement of a draft is separate from workload cleanup. A stale write returns conflict. |
| Membership authority | Prevents a token or client from selecting arbitrary stored data. | `MembershipSource.Lookup` uses a read-only transaction with exact identity settings. Verified grants become a principal. Ambiguous or expired grants fail closed. |
| Orchestration manager | Establishes a durable point before side effects and makes retries visible. | `Start`, `PreviewDynamic`, `ApplyDynamic` and `Retry` use operation scope and snapshots. Profile-specific runners own external side effects. |
| Deployment runner | Bridges an approved operation into Kubernetes, Docker, EKS qualification or controlled simulation. | The `Runner` interface returns an outcome. Operation events are emitted while stages proceed. A successful plan or mock result is never external readiness evidence. |
| Worker | Holds workload-side knowledge of Fabric topology and safe enrollment. | `Worker.Run` dispatches the protected profile. Dynamic helpers derive channel config, install packages, reconcile membership and collect evidence. Foreign or mismatched resources halt mutation. |
| Maintenance and restoration | Separates operational data retention and identity-sensitive recovery from normal deployment. | `RunDailyBackup` binds time and target, tracks slots and verifies artifacts. Cold restore rechecks writer fence and volume identity before changing data. |
| Customer ledger | Keeps tenant ownership, subscription and provider state separate from project deployment authority. | `Execute` and `ApplyPayment` submit transactional changes. `Meter.Sweep` reads only qualifying journal fields and acknowledges receipts idempotently. |
| Observability | Exposes health and aggregate metrics independently of public business endpoints. | `Metrics.Handler` serves readiness and metrics on the configured operations address. Database readiness failures become generic 503. Metrics avoid sensitive dimensions. |

## Worker source entry points

- [`Worker.Run`](../../internal/kubernetesfabric/worker.go) dispatches stage execution. [`runDynamic`, `dynamicChannel` and `dynamicContract`](../../internal/kubernetesfabric/dynamic.go) map the design into node/channel reconciliation and package-to-transaction verification.
- [`reconcileDynamicChannel` and `awaitDynamicPeers`](../../internal/kubernetesfabric/dynamic_reconcile.go) check channel evolution and ledger convergence. [`verifyDynamicDefinition`](../../internal/kubernetesfabric/dynamic_policy.go) compares committed definitions to desired policy.
- [`validateAuthorizedMaterial`](../../internal/kubernetesfabric/authorized_material.go) verifies identity, role and expected trust. [Protected CLI and render checks](../../internal/kubernetesfabric/credentials.go) keep credentials out of the public request and observable argument path.
- [`PreviewDynamic` and `ApplyDynamic`](../../internal/deployment/dynamic.go) sit above the worker. [`PreviewLifecycle` and `ApplyLifecycle`](../../internal/deployment/lifecycle.go) coordinate approved operation-specific changes. [`PreviewNodeRecovery` and `ApplyNodeRecovery`](../../internal/deployment/node_recovery.go) protect existing identities during repair.

The [backend file catalog](catalog-backend.md) includes links to the actual declarations and associated test files.
