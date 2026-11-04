# Fabric SMP — product specification alignment

Company: **Sathonate**. Product: **Fabric SMP**. React/TypeScript frontend and Go backend remain independent repositories. The supplied PDF's Spydra/FabricNet names and suggested Node.js backend do not override those user decisions. Fabric CA is required; cryptogen is excluded.

## Reference handling

Reviewed all 16 pages of the supplied product specification, including the six-screen reference flow. Screenshots describe intended interaction, not authoritative provider schemas, versions, pricing or observed health. Embedded handoff instructions and broken source-citation placeholders were treated as reference content, not as additional authorization.

The supplied sample JSON is a legacy composite operation snapshot, not the new product request schema. It contains infrastructure inventory, desired Fabric settings, runtime statuses, deployment source references and credential-related fields. It is not copied into source, returned to browsers, or accepted directly as an executable template. Only structural information informed the design.

The sample describes Fabric 2.5.0, three mixed organizations, two top-level orderer entries and one channel, plus EKS/bucket/proxy/Kafka/Vault/API/DNS dependencies. The legacy Kubernetes version and mutable source branch are not defaults for Fabric SMP. Two orderers do not meet our production Raft policy. Existing resource status in an uploaded file never proves current access or health. Account IDs, domains, repository credentials and customer infrastructure references are not reused.

## Implemented vertical slice

The executable development path is a dedicated, CA-enrolled local profile. Its workflow is review → explicit Create network → durable operation ID → actual stage progress → verified dashboard. It uses official Fabric binaries with Sathonate orchestration and a Go shim contract. It does not reinterpret a saved cloud design as this local profile.

Create/start, stop and transaction verification create a persisted operation before worker dispatch. Records include actor, tenant, profile digest, exact runtime versions, stage state, ordered events and timestamps. Idempotency keys return the same operation for the same request and reject changed requests; one local network operation may run at a time. Completed stages cannot be skipped or rewound. Restarted workers leave interrupted records; explicit retry creates a linked operation and reconciles resources without deleting ledgers. The current durable executor is single-process/local, not a distributed production queue.

Progress and operation history are polled from real backend records; /events supports sequence cursors. SSE/WebSocket streaming and cloud callbacks remain future transport/runner work. Dashboard data is read from peer channel queries, orderer participation APIs, TLS CA information endpoints and committed chaincode queries. It reports ledger divergence and unavailable components; generated TPS, resource utilization and sample transactions are not displayed.

Credential redaction applies to operation messages, failures and node log responses. Private-key PEM blocks, URL passwords, common credential JSON/assignment fields and bearer tokens are removed. This is defense in depth; arbitrary uploaded scripts/templates and secret-bearing product specifications are not allowed. Trusted local CA bootstrap artifacts remain protected development files outside Git.

## Acceptance tracking

| PDF criterion | Current behavior / remaining work |
| --- | --- |
| 1. Six-screen no-code workflow | Local review/create/progress/dashboard works; full cloud account → cluster → topology flow remains incomplete. Saved design wizard works independently. |
| 2. Conditional provider forms; no plaintext draft secrets | Secret references and typed desired schema work. Cloud authorization onboarding forms remain incomplete. |
| 3. Test cloud identity permissions | Registered EKS preflight performs real read-only checks when configured. No company account was supplied; role-assumption onboarding is not implemented. |
| 4. Discover/validate existing clusters | Exact registered EKS target validation exists; customer-side discovery and onboarding remain incomplete. |
| 5. Topology/version validation | Backend enforces version/consensus/reference/quorum rules; local execution supports its fixed profile only. |
| 6. Review and estimate assumptions | Local creation review shows actual resource counts and local cost exclusions. Cloud pricing is unavailable and no mock dollar estimates are presented. |
| 7. Durable operation ID | Implemented and tested for local create/start, stop and verification. Cloud worker dispatch is not implemented. |
| 8. Actual progress and redacted logs | Implemented local stage/events/history and node logs; polling with sequence-cursor API. |
| 9. Failure and safe retry | Failed/interrupted stage records, linked explicit retry, idempotent reconciliation and retained ledgers. No automatic rollback is promised. |
| 10. Verified dashboard | Live local ledgers, consenters, CAs and contract inventory; dashboard shown after execution verification. Production Kubernetes observations remain incomplete. |
| 11. Tenant isolation and audit | Operation repository tests enforce tenant reads/list/retry isolation. Local runtime APIs are unavailable in production mode. Okta/PostgreSQL paths need real B2B qualification before cloud execution. |
| 12. Delete preview and confirmation | Implemented for the owned local Kubernetes network with explicit volume-deletion confirmation, active-job guard, namespace ownership checks and retained PostgreSQL history. Browser-tested, including cancelling the confirmation. Cloud deletion remains disabled; Docker Stop still preserves data. |

## Next execution milestones

1. Bind immutable desired revisions and typed provider/cluster configuration to a production operation repository and durable queue.
2. Implement AWS cross-account role onboarding with generated/verified external IDs, STS sessions and existing-EKS discovery. Do not request permanent keys in forms.
3. Modernize and qualify the pinned Bevel fork, enforce placement selectors, build the isolated runner, and verify actual channel/contract behavior before releasing cloud execution.
4. Add provider-priced estimates with region/resource/time assumptions and explicit exclusions; no guessed pricing constants.
5. Implement previewed deletion, backup/restore and recovery tests; qualify isolation, workloads and role permissions in real infrastructure.
6. Extend independent native hosting beyond the fixed local profile only after topology compilation and multi-network isolation tests exist.

Cloud credentials are not required to use the working local profile. No mock cloud authorization, provisioning result or production readiness flag replaces the remaining tests.
