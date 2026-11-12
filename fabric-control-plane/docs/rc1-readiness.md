# RC1 release readiness

**Overall BLOCKED for production.** Qualification started from `5647ef9`. Sprints 6–9 remain PARTIAL. Existing sprint reports and evidence are preserved. RC1 did not launch kind, query or change the retained cluster, rotate credentials, call a live payment provider, or provision paid services.

## Customer journey

| Step | Available and verified | Remaining gate |
| --- | --- | --- |
| Sign in, select account and project | The existing Okta PKCE/MFA configuration and server-owned grants are exercised with a signed issuer fixture. The browser pilot exercises trial and invitation workflows. | Real Okta browser acceptance, greenfield project identity onboarding, and independent exact-project grants. |
| Start trial, create project, plan target | Trial, catalog project and invitation state persist in PostgreSQL. RC1 adds `POST /api/v1/networks/{id}/customer-preflight` for a saved customer Kubernetes design. It checks the active project, exact draft revision, approved target, storage classes and Raft topology. The receipt binds the desired configuration and namespace UID with execution disabled. | Customer target registration and RBAC must be proven on an approved external namespace. The target registry is a trusted operator inventory. It does not probe the live cluster. |
| Deploy and expand | Existing local API-to-Fabric evidence remains available. Customer Kubernetes designs cannot enter the current executor. Entitlement now applies to ordinary deployment retry as well as create and expansion. | Customer-owned Kubernetes actuator, ownership fencing and exact target revalidation at admission and reconciliation. |
| Observe, recover and retire | Scoped operations, audits, recovery and retirement use existing approvals. RC1 adds aggregate entitlement decisions on the separate metrics listener. Cleanup and recovery retain their own authorization during billing interruption. | Actual scrape and alert delivery, cross-worker failover, replacement-target recovery and production operating procedures. |
| Meter and manage billing | RC1 adds the scoped `fabric-customer-meter` job command. It reads confirmed journal receipts, deduplicates in PostgreSQL, resumes via bounded replay, and reports aggregate outcomes. Controlled payment and portal adapters are preserved. | No currently enabled executor produces customer Kubernetes receipts. Scheduler deployment, catch-up policy, provider integration and actual settlement are open. Customer-owned infrastructure remains outside Sathonate billing. |

## Launch blockers and ownership

Owners below are accountable roles. A release coordinator must assign named owners and verify the review records. **P0** blocks any advertised production launch. **P0 paid** blocks any paid launch. **P1** blocks the affected operating capability and must be explicitly removed from the supported release scope or passed.

| Class | Severity | Owner | Exact blocker and prerequisite |
| --- | --- | --- | --- |
| Software defect fixed | P0 | Control-plane engineering | Deployment retry previously bypassed the pilot entitlement check. Ordinary retries now require the same active project. Recovery of existing workloads retains its separate authorization. |
| Missing integration | P0 | Identity engineering and customer admin | Greenfield projects cannot complete self-service identity provisioning. The existing production grant needs an approved network anchor and the server-bound policy selects a single active grant. Qualify a distinct approved project bootstrap and project selection flow with positive and negative tenant tests. |
| Missing integration | P0 | Platform and Fabric engineering | No qualified customer-owned Kubernetes actuator. Implement scoped access and reconciliation against the preview's exact binding, fenced ownership, identity enrollment, CSI/placement checks and post-change ledger verification. Run scenario K8S below. |
| Missing integration | P0 paid | Billing engineering and Finance | No provider-specific sandbox or production adapter is registered. Select the provider, implement signature normalization, hosted checkout, retries, ordering, refunds/reconciliation and invoice settlement. Approve catalog prices and billing support procedures. Run scenario PAY. |
| Missing integration | P0 | Fabric operations and security | Replacement-target recovery requires portable identity/volume mapping, independent target validation, encryption key custody and single-writer cutover. Same-target cold restore cannot satisfy this requirement. Run scenario DR with approved RPO/RTO. |
| Infrastructure prerequisite | P0 | Customer Kubernetes owner and SRE | Provide a disposable customer-owned target, pinned images, approved ingress TLS, restricted PostgreSQL, real CNI/CSI, isolated namespace and cleanup privileges. Qualify the four-node capacity gate on an approved host or lab. Run K8S and MULTI. |
| Infrastructure prerequisite | P0 | SRE and release engineering | Prove alert delivery, audit collection and retention, verified storage recovery, operational pager ownership and final-source load and soak. Adopt the Sprint 8 API thresholds and approve separate Fabric throughput and recovery objectives before measurement. |
| Security approval | P0 | Okta admin and test account owner | Approve the actual SPA redirect and logout origins, policy assignment, MFA enrollment and read/write scopes. Complete browser sign-in and token assurance evidence. Run scenario OIDC. |
| Security approval | P0 | Incident owner and Fabric org/channel owners | Historical credential exposure lacks confirmed affected-fingerprint inventory, authorization for per-class rotation and independent rejection/transaction evidence. Follow the existing remediation plan with explicit owner approval. |
| Security approval | P1 | Fabric release and channel owners | Live TLS/MSP rollover and peer/orderer/CA upgrades await approved image transitions, exact quorum/endorsement checks, backup and rollback or forward-recovery evidence. |
| Production activation | P0 | Release manager, security, SRE and customer owner | Separate written decisions for customer execution, production Okta policy and production operations. Review final image digests, independent receipts, incident closure, SLOs, rollback, cleanup and on-call. |
| Production activation | P0 paid | Finance, security and release manager | Separate written payment activation after provider sandbox passes, reconciliation and refund operations are rehearsed, and monitored rollout is approved. |

## Execute qualification

Use one exact candidate Git SHA and image digests throughout. Keep redacted artifacts with UTC timestamps, exact target ID fingerprints, outcome, transaction IDs and ledger heights where relevant. Never put credentials, bearer tokens, authorization codes, session URLs, webhook signatures, raw card data, or private keys in receipts. Verify the owner-approved disposable target and teardown before any mutation. Stop on target, identity, quorum or ledger mismatch.

- **K8S**. Use an approved disposable customer cluster and its explicit kubeconfig and namespace. Render and validate both Helm profiles with `python -B tools/sprint8/check_helm.py --helm helm`. Once the customer actuator is approved, onboard two independent tenants through the API, save a dynamic Raft topology, and POST its exact revision to `/api/v1/networks/{id}/customer-preflight`. Require the wrong revision and foreign target to fail. Deploy using the approved actuator and replay the same request key. Expand the topology, commit and query a fresh transaction from every required peer, verify TLS/MSP and CNI denial from the other tenant, inject a controlled workload restart, verify CSI/PVC identities and perform approved cleanup. Run the meter twice and check one usage entry for a qualifying operation and no infrastructure charge. Scrape the operations listener from the approved Prometheus identity, deny other ingress, and check alert delivery.
- **OIDC**. Use the deployed pilot HTTPS origin and the approved Okta SPA. In a clean browser, sign in via PKCE and MFA, verify callback and authorized reads, attempt writes before step-up, then authorize the write scope and repeat as an owner. Test a viewer, a foreign tenant/project, invalid issuer/audience, expired token and missing scopes using controlled negative requests. Sign out, verify browser session handling and separately verify the expected token revocation/expiry policy. Record only redacted status and assurance metadata. A signed local issuer fixture is PARTIAL for this gate.
- **PAY**. Once a provider-specific sandbox adapter is approved, use the sandbox hosted checkout and portal. Test success, payment failure, cancel/refund and invoice reconciliation. Redeliver the identical signed event and send a later delivery for an older event. Confirm one financial transition, customer binding, recorded audit, and appropriate deployment entitlement. Test a wrong customer, invalid signature and foreign tenant. Verify no card data enters Sathonate. Keep real payment mode off.
- **MULTI**. On a separately approved adequate host, run `powershell -File tools/sprint6/qualify-local.ps1`. This invokes the existing resource-gated disposable worker harness. Its host gates require at least 8 GiB Docker total, 6 GiB Docker available and 8 GiB Windows available. Verify cross-worker transactions, TLS, surviving Raft quorum, worker pause/recovery, stateless rescheduling, original local-PV identity, ledger continuity, three separate timings and cleanup. A single-host kind run cannot establish independent physical-host fault tolerance.
- **DR**. After resource approval, run `python tools/sprint7/qualify.py --run --output .logs/rc1-same-target.json` to qualify scheduled encrypted capture, restore and transaction checks on original storage. Separately restore PostgreSQL and Fabric to an independently approved empty replacement target when the portable adapter exists. Check wrong key, foreign target, divergence and concurrent writer rejection. Demonstrate the restored ledger and new transactions, measure RPO/RTO against preapproved objectives, switch traffic under the cutover fence, and verify cleanup. Original-storage success remains PARTIAL for replacement recovery.

## Receipts and exit criteria

Run `python -B tools/rc1/check_receipts.py --list` to get exact check IDs. Store one `<gate>.json` per gate beside its redacted artifacts, using the following format. Run `python -B tools/rc1/check_receipts.py --directory <receipt-directory> --candidate <full-git-sha>`.

```json
{
  "gate": "okta",
  "candidate": "<full-git-sha>",
  "environment": "live",
  "reviewRef": "approved-review-reference",
  "checks": {"pkce-mfa-callback-logout": "PASS"},
  "evidence": [{"file": "okta-redacted.txt", "sha256": "<artifact-sha256>"}]
}
```

**PASS** requires every named check to pass in a live, candidate-bound receipt with artifact checksums and independent review. **PARTIAL** covers controlled fixtures or incomplete live evidence without a known failed check. **BLOCKED** covers missing prerequisites or receipts, failed live checks, invalid evidence or missing review. The checker verifies receipt structure and hashes. Reviewers must independently verify evidence provenance, test execution and the authorization of the review reference. It cannot approve a release. All five live receipts are currently missing.

## Software verification

- **PASS**. Fresh Go tests and vet. Targeted signed OIDC tests covered revision-bound target preview, denied expired-entitlement retries and execution-disabled customer designs. Metric tests reject unbounded labels.
- **PASS**. The disposable PostgreSQL 17.6 TLS fixture verified forced row security and read-only journal grants for the meter. Two sweeps over one qualifying synthetic receipt produced one usage entry. The fixture excluded a foreign-scope receipt and a local qualification receipt. An interrupted two-page sweep resumed and acknowledged all 101 receipts exactly once in billing.
- **PASS**. Both Helm profiles passed strict validation. The opt-in customer inventory rendered with its read-only mount and alert rule. Inventory without the pilot was rejected. The existing chart still renders execution disabled.
- **PASS**. The console passed 31 unit tests, the build and three intercepted Edge browser scenarios. The build retained the existing large-chunk warning.
- **PASS**. Receipt tests covered missing artifacts, checksum changes, candidate mismatch, partial fixtures and review references. All five live gates evaluated BLOCKED with no live receipts.

## Operate the software checks

The optional API flag `-customer-targets <protected-json>` requires the authenticated pilot and disabled execution. With Helm, set `customerPilot.enabled=true` and provide an existing same-namespace ConfigMap via `customerPilot.existingTargetInventoryConfigMap`. Give the ConfigMap a `targets.json` key and restrict who can edit it. The Helm default leaves the pilot off. Supply an array of exact `ref`, `tenant`, `project`, `clusterSHA256`, `namespace`, `namespaceUid`, and `storage` entries. Storage capability fields are `Provisioner`, `Encrypted`, `WaitForFirstConsumer`, and `MaxCapacityGiB`. The saved design must use Bevel, Kubernetes, `provider: customer-kubernetes`, and the approved `targetRef`. Send the existing bearer authorization, JSON body `{}`, and `If-Match` containing the current draft revision.

Use a separate verified-TLS meter connection in `CUSTOMER_METER_DATABASE_URL`. Grant it the restricted customer runtime column permissions and SELECT-only on operation and deployment journals. It must not inherit the executor role. Schedule `go run ./cmd/fabric-customer-meter -tenant TENANT -project PROJECT -window 168h` for the explicitly approved project. The job exits nonzero if the sweep is incomplete. Retry the same scope and window. The report counts reviewed and acknowledged receipts, including idempotently replayed receipts. For a longer outage, widen the window within 30 days and reconcile older receipts through approved manual backfill. No production scheduler was activated. Alert on nonzero job exit and on `sathonate_customer_entitlement_checks_total{result="unavailable"}`; the metric carries no tenant, project or operation labels.

[Sprint 6](sprint6-acceptance.md) · [Sprint 7](sprint7-acceptance.md) · [Sprint 8](sprint8-acceptance.md) · [Sprint 9](sprint9-acceptance.md) · [Historical remediation](sprint7-historical-credential-inventory.md)
