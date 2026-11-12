# 13 · Architecture decisions and glossary

[Handbook](README.md) · [System design](01-system.md)

These are retrospective summaries of observed source decisions. Repository change history remains the authority for original approval context.

| Decision | Rationale and consequence | Evidence |
|---|---|---|
| Separate intent, observations and jobs | Protects optimistic design revision and makes partial execution visible. Operational success needs verified outcome evidence. | [API](../../internal/httpapi/api.go), [journal](../../internal/operations/store.go) |
| Bind authorization server-side | Tenant/project memberships come from approved storage. OIDC claims prove identity and constrain scopes. | [OIDC](../../internal/identity/okta.go), [membership storage](../../internal/persistence/oidc_memberships.go) |
| Keep production execution off | Preserve qualified local paths while customer actuator, trust and recovery are under qualification. | [Startup](../../cmd/fabric-api/main.go), [RC1](../rc1-readiness.md) |
| Fence destructive and irreversible operations | Namespace UID, resource identity, approvals and quorum reduce stale or foreign mutation. | [Recovery](../../internal/deployment/node_recovery.go), [upgrade](../../internal/rollout/upgrade.go) |
| Separate customer billing from infrastructure | Usage counts verified platform operations; provider receives opaque references. | [Customer metering](../../internal/customer/usage.go), [provider contract](../../internal/customer/hosted.go) |

| Term | Meaning |
|---|---|
| CA / MSP | Enrollment authority / membership service provider configuration and identity trust. |
| PKCE / JWKS | Authorization-code proof binding / public keys for token verification. |
| Raft consenter | Voting orderer listed in a channel configuration. |
| Endorsement | Required peer signatures over a transaction proposal result. |
| Chaincode sequence | Versioned channel definition progression, distinct from package hash. |
| RLS | PostgreSQL row-level security enforced in addition to API permissions. |
| PVC / CSI / CNI | Persistent volume claim / storage interface / network interface. |
| Fencing | Restriction preventing stale or concurrent writers from acting on the target. |
| RPO / RTO | Approved data-loss and restoration-time objectives. |
| Receipt | Redacted observation or acceptance result bound to the candidate, target and workflow. |
