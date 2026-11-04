# Security findings and remediation matrix

Statuses describe observed code/test evidence, not a production certification. No live AWS environment or Okta tenant has been qualified.

| Finding | Status | Implemented control / next requirement |
|---|---|---|
| Enrollment passwords in OS argv | Locally verified reduction | Protected stdin CLI and Docker private-config delivery; real local transactions and CA registration tested. Privileged process memory remains trusted. |
| Organization password reuse | Locally verified reduction | Ten independent persisted identity credentials in supported fresh networks; retries preserve values; no silent legacy rotation. |
| Inline CR enrollment fields | BLOCKED migration | CA registry `pass`, peer/orderer component/TLS `enrollsecret`, chaincode TLS credentials remain sensitive; read permissions, Helm histories and audit/backups must be restricted. |
| External certificate adoption | NOT VERIFIED | Pinned peer/orderer validators require enrollment secrets in Kubernetes credential mode even though external certificate fields exist. Vault is a different store requiring new dependencies/identity design; not configured or installed. |
| Controller cluster privileges | BLOCKED shared-tenant release | Controller can create cluster channels and bootstrap RBAC; worker restriction is not controller containment. Prefer dedicated qualification cluster and reviewed operator/controller identity. No assertion of effective CNI/admission isolation. |
| Cleanup namespace-wide deletion | Local UID transport verified | Cloud broad deletion disabled; administrator preview, exact names/UID preconditions, ownership and binding checks implemented. Local development cleanup preserved. Real kind UID denial/unrelated-probe preservation passed; operator cascading/data retention remain unverified. |
| Rotation/renewal/revocation | NOT VERIFIED live operation | Implement planning/state/recovery components without wiring an unqualified credential mutator. No existing credential is automatically changed. |
| Backup confidentiality | Local primitive verified | Authenticated encryption and binding/integrity/no-overwrite private outputs tested; live PVC/database consistency, recovery point and KMS recovery remain unqualified. |
| Secrets/PVC retention | Explicit retained boundary | Workload cleanup is not data destruction; retained volumes, wallet keys, Helm release data and snapshots require a separate owner-approved lifecycle. |
| Git/source provenance | Reviewed initial baseline | Initial commit `e378cbf66fa74f3c533231830ec9225ce3aaa353`; follow-up work has a separate dirty-tree source hash. Signed release/image provenance remains unverified. |
| AWS EKS, EBS/IAM, AZ recovery | NOT VERIFIED | Explicit account/role/region/cluster, budget and workload approvals required; no AWS execution performed. |
| Public enterprise access | BLOCKED | Qualification is loopback admin-only. Live Okta positive/negative roles and tenant denial remain prerequisites. |

## Milestone 2: pinned compatibility determination

The reference checkout is read-only and pinned to `cd236ccc61d1d159b3dadb4625d08bb5682a8451`. Peer `ConfigValidator.validateCredentialStore` chooses Kubernetes configuration by default, where `validateFabricCAConfig` requires a CA host, enrollment ID and enrollment secret. The orderer validator does likewise. `Component.External` / `TLSComponent.External` contain certificate/private-key Secret references, not password references. Those read paths do not bypass the initial Kubernetes-store validation. CA registry identities use inline passwords; CA signing-key Secret support is not enrollment-password Secret support.

The supported Vault path requires configured component/TLS Vault data and a separately qualified security lifecycle; no Vault deployment or live controller replacement was performed. The current follow-up tests a validation-only candidate in a separate pinned archive; this does not qualify reconciliation. Public upstream features/renewal documentation is useful background but does not prove this exact patched build supports a password-free lifecycle: https://github.com/hyperledger-bevel/bevel-operator-fabric and https://hyperledger-bevel.github.io/bevel-operator-fabric/ . No newer controller revision was adopted.

Smallest viable subsequent change: isolate a pinned controller patch that validates pre-enrolled signing/TLS Secret references without requiring passwords, enforces same-namespace references, and consistently reconciles peers/orderers/chaincode plus renewal. Test fresh enrollment, retries, certificate replacement, ledger/consenter recovery and rollback on disposable kind before adopting it. CA bootstrap storage must also be addressed; clearing a field after initial reconciliation does not solve Helm history or backup exposure. This migration remains BLOCKED, not implemented behind an invented field.

Until then treat Fabric CRs, operator state, CA databases, Helm release Secrets/history and audit/backup payloads as credential stores. Limit access to approved controller/worker identities, use metadata-only audit for those resources, disable payload collection in diagnostics, encrypt Kubernetes storage/backups, restrict log aggregation, and avoid shared tenant clusters pending containment tests. No policy object is counted as effective enforcement merely because it exists.


## Current follow-up evidence

- Pinned peer/orderer password requirement proven by actual upstream validator tests. The isolated validation-only candidate accepts complete same-namespace external references and rejects foreign/incomplete references; no controller replacement was adopted.
- Real Docker CA issuance and reenrollment plus renewed certificate/key Secret-reference validation pass. Full password-free Fabric lifecycle remains BLOCKED. Actual retained CR inspection finds populated password fields.
- Forced RLS added to operation/deployment journals; real restricted-role own-tenant and cross-tenant denial tests pass. Explicit executor capability required at recovery startup. The trusted executor retains cross-tenant access, and production role separation is not yet qualified.
- Installed controller can read Secrets cluster-wide and create ClusterRoleBindings. Live permission reduction is BLOCKED pending a separately qualified controller/channel boundary; worker restrictions do not mitigate controller compromise.
- Exact commands, failed-attempt explanations and identity/renewal/cleanup limitations: `passwordfree-isolation-milestone.md`. Current redacted results: `.logs/milestone-evidence.json`. AWS qualification remains false.


## Controller candidate follow-up

| Finding / gate | Current outcome |
|---|---|
| Verified prior milestone committed | PASS — `4457e8e894f53fb7b5e0cad4e7cdee1aa8c6ce9e`; staged review found no credential patterns. |
| Upstream orderer private-key logging | FAIL — statement exists at the pin; actual retained-controller log sample contains private-key markers. Raw logs were withheld. No retained identity rotated. |
| Candidate logging | Partial control — removes key-print statement, discards upstream logrus; actionable secret-safe diagnostics still require work. Not deployed to retained network. |
| Namespace-scoped candidate | Actual negative RBAC probes verify foreign Secret/Fabric reads, cluster Secret listing, ClusterRoleBinding creation and foreign cleanup denied. Candidate has only a namespace Role; no cluster channel controller. |
| Complete password-free Fabric lifecycle | BLOCKED — CA registry Secret-to-runtime configuration bridge, auxiliary TLS issuance/recovery and channel broker separation are incomplete. Rejected password-free CR fixtures do not count as an operational Fabric network. |
| Certificate replacement | Unit-tested new material/trust/SAN/key validation only; live renewal, consenter/admin recovery and interrupted certificate mutation NOT VERIFIED. |
| Existing controller containment | BLOCKED — retained controller privileges and history unchanged, including cluster-wide Secret access and potentially sensitive logs. |

See `controller-security-candidate.md` for exact technical blockers, viable alternatives, source patches and reproducible commands. Treat exposed historical operator logs as credential-bearing material; owner-approved incident assessment and coordinated rotation are prerequisites, not actions silently performed by this task.

Final isolated kind probe: all 14 security checks PASS, cluster cleanup PASS. Full replacement Fabric lifecycle remains BLOCKED; stored rejected-fixture password absence is not operational-network evidence. Tested digest and exact final results are recorded in `enterprise-readiness.md` and `.logs/controller-final-evidence.json`.


## CA registry bridge follow-up

The candidate now implements the Secret-to-memory CA registry bridge, an explicit patch to the pinned controller and chart, not an upstream CRD feature. Root/TLS credentials differ, chart values contain references only, and public registries remain empty. Actual CA-only verification is recorded separately in controller-registry-followup.md and .logs/controller-registry-kind.json. This supersedes only the prior bridge-not-implemented statement, not the full lifecycle blocker.

Remaining blockers: original controller cluster-wide Secret privileges and historical key logging; CA private keys in Helm storage; namespace-level Secret access; no effective admission/CNI isolation qualification; incomplete auxiliary/channel/chaincode issuance, interrupted reconciliation and recovery. Registrar rotation and CA database recovery are not qualified. Working engine/controller and retained credentials are unchanged. AWS qualification is still blocked.


## Secure controller lifecycle follow-up

See controller-lifecycle-qualification.md for the current candidate result matrix, exact commands and unresolved security limits. Its runtime evidence supersedes earlier implementation gaps only for explicitly passed operations. The retained controller, historical key logging and original Helm history are unchanged. Namespace-wide Secret access, effective admission/CNI isolation, coordinated Raft TLS rotation, CA/registrar rotation and actual EKS qualification require separate acceptance evidence. No production adoption or AWS execution is enabled.

Latest qualification (23 October 2026): real disposable candidate lifecycle PASS, nine stages / 42 checks, including actual empty enrollment-password CR fields, private material absent from inspected Helm/runtime resources/logs/argv, peer TLS renewal during controller outage, committed transactions on both peers and disposable cleanup. Root/targeted Go, PostgreSQL, Docker CA, both original kind regressions, protected CLI and redaction tests PASS. Retained deployment readiness 8/8. Original-controller key logging remains FAIL. Enterprise promotion, broader identity/CA/Raft rotation, partial-write failure recovery, effective CNI/admission isolation and AWS qualification remain BLOCKED / NOT VERIFIED. Milestone 2 is not fully complete; see the qualification report for exact scope and evidence.


## Atomic peer-TLS recovery follow-up

See [controller-atomic-qualification.md](controller-atomic-qualification.md) for final source-linked evidence. PASS: all six real disposable failure boundaries, nine-stage/42-check baseline, live certificates and committed/query transactions on both peers, final public-resource/Helm/log/argv inspection and cleanup. Post-rollout acknowledgement recovery independently preserved Helm revision 13, peer pod UIDs and container restart counts. This supersedes the partial-write recovery gap only for the isolated peer-TLS candidate. Historical original-controller key exposure, production integration, effective tenant isolation, broader Fabric identity/Raft/CA rotation and immutable-bundle retention remain blockers. Production and AWS remain disabled.
