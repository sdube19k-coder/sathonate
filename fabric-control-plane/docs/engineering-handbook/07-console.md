# 07 · Console

[Handbook](README.md) · [API](05-api.md) · [Commercial](10-commercial.md)

The Vite entry `main.tsx` mounts `AuthGate` and `App`, which delegates to `SmpApp`. The Go static handler serves the SPA and callback. App navigation uses component state and selected hashes. The six-step flow covers project/profile, Kubernetes configuration, network design, review, deployment progress and network dashboard. Progress and health read server state. Networks, nodes, chaincode, operations, cloud readiness and customer configurations are workspace views.

`NetworkVisualBuilder` and the configuration/model helpers construct desired topology. `CustomNetworks` manages supported customer-defined designs and expansion. `SmpRuntime` and `SmpScreens` render deployment and dashboard surfaces. `WorkloadStatus`, `NodeRecovery` and `RetainedDeletion` present observed workloads and guarded maintenance. `CustomerAccount` is capability-gated and calls the authenticated pilot API. The RC1 customer target preview has an API contract; a dedicated customer-target selection journey still needs integration.

`workspace.tsx` supplies project context. Draft continuity uses workspace-keyed session data while bearer tokens remain in memory. The API transport attaches authorization and raises structured API errors. Authentication expiry returns the local bearer UI to sign-in. Read-only history and retired operations must not present live execution as available. Retrying a customer mutation in the same view retains its key after ambiguous failures.

[Entry](../../../fabric-console/src/main.tsx) · [App](../../../fabric-console/src/SmpApp.tsx) · [API transport](../../../fabric-console/src/api.ts) · [Visual builder](../../../fabric-console/src/NetworkVisualBuilder.tsx) · [Transport tests](../../../fabric-console/src/api.transport.test.ts) · [Pilot browser cases](../../../fabric-console/e2e/customer-pilot.spec.ts).

## Console component contracts

| Component | Input and output |
|---|---|
| `AuthGate`, `LocalSignIn`, `ProjectGate` | Fetch auth config, establish the selected authorized project and pass its context to descendants. The browser never selects tenant permissions on the server. |
| `SmpApp`, `SmpScreens`, `OrderingStep` | Coordinate workspace views, form validation, workflow steps and review. The selected profile controls which controls appear. |
| `NetworkVisualBuilder`, `visual-network`, `CustomNetworks`, `custom-configuration` | Build or import a typed topology, preserve advanced references and present change previews. An import or edit does not provision resources. |
| `SmpRuntime`, `OperationProgress`, `OperationDiagnostics` | Render recorded stage and event data and expose bounded diagnostics. Use the persisted operation when reconnecting. |
| `RuntimeDashboard`, `WorkloadStatus`, `NetworkDetail` | Present observed inventory and network status separately from the saved design. |
| `NodeRecovery`, `RetainedDeletion`, `LifecycleManagement` | Present approved preview, confirmation and reconciliation state. Hide or restrict actions when history is read-only or a deployment is retired. |
| `CustomerAccount` | Shows organization, invitations, trial, projects and owner-only invoice and usage information. Requests write step-up through Okta when needed. |
| `CloudReadiness`, `LaunchReadiness` | Show readiness prerequisites and preview/assessment data. They do not enable live cloud operations. |
| `api`, `types`, `workspace`, `read-recovery` | Shared request transport, response shapes, authorization-aware workspace selection and read recovery. |

Use the [console catalog](catalog-console.md) for each source file, test and direct relative import.
