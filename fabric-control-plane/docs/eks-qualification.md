# EKS qualification harness

The harness is implemented for individual authorized actions, not unattended production launch. Default `plan` is offline. Every other action needs `-allow-cloud-read`; mutations additionally require `-execute`, a budget/change reference and a current per-action approval bound to exact target/network hashes. No action provisions EKS, IAM, dependencies, DNS, load balancers or storage classes. Deploying Fabric may create billable PVC-backed volumes; do not execute without authorization.

## Prerequisites

Use the exact supported Fabric 2.5.15/CA 1.5.15/two-org/two-peer/three-Raft/one-channel profile, pinned operator/CRDs, protected CLI worker image, verified registry digests, three approved EKS 1.34 AZ placements and encrypted delayed-binding EBS CSI storage. Administrators must separately create and approve the cluster dependencies and exclusive namespace, policies, quotas, credentials, controller access and dedicated PostgreSQL database. See `eks-worker.md` and `eks-readiness-verifier.md`. No environment is assumed to exist.

Require a reviewed committed source baseline before qualification. The current untracked implementation/content hash is not release provenance. Configure a role-backed SSO profile and explicit kubeconfig using the existing runbook. Store all configuration and approval files in an administrator-owned private directory. Inject database connection strings through a secret facility; never supply them as command arguments.

## Ordered commands

```powershell
Set-Location E:\Sathonate\fabric-control-plane
go build -o bin/fabric-eks.exe ./cmd/fabric-eks
go build -o bin/fabric-api-eks.exe ./cmd/fabric-api
go build -o bin/fabric-acceptance.exe ./cmd/fabric-acceptance
# These named parameters are reused below. Files contain no inline credentials.
$common=@('-registry','<private-targets.json>','-network','<saved-network.json>','-tenant','local-development','-network-id','<saved-record-id>','-target','company-eks')
./bin/fabric-acceptance.exe -action plan @common -output <new-plan-evidence.json>
# Only after explicit authorization for existing-cloud reads:
./bin/fabric-eks.exe -action readiness @common -check-postgres -evidence <new-readiness.json>
./bin/fabric-acceptance.exe -action preflight @common -allow-cloud-read -output <new-preflight.json>
```

`readiness` intentionally exits 2 for BLOCKED or 3 for NOT VERIFIED; do not bypass unresolved security checks. A successful access preflight is not production qualification. The qualification API must be separately started on loopback as documented in `eks-worker.md`, using its exact-target deployment approval and dedicated migrated database. Save the network draft first; the harness requires its exact desired specification and record ID.

Per-action approval file is an array, with separate reviewed entries for each authorized mutation:

```json
[{"action":"deploy","targetSHA256":"<target hash>","networkSHA256":"<network hash>","authorizationRef":"approved-change","expiresAt":"<UTC timestamp within one hour>"}]
```

```powershell
# Execute only after environment/budget/workload approval, not as a setup step.
./bin/fabric-acceptance.exe -action deploy @common -allow-cloud-read -execute -budget-authorization <approved-budget-id> -action-approvals <private-action-approvals.json> -idempotency-key <stable-request-key> -output <new-submit.json>
# Record operationId returned by submission. Duplicate submission is checked by the harness.
$op='<returned-operation-id>'
./bin/fabric-acceptance.exe -action observe @common -allow-cloud-read -operation $op -output <new-observation.json>
./bin/fabric-acceptance.exe -action storage @common -allow-cloud-read -operation $op -output <new-storage.json>
./bin/fabric-acceptance.exe -action isolation @common -allow-cloud-read -operation $op -output <new-rbac.json>
```

Observe until actual completion; the harness does not equate submission with success. It records current workload health separately from the timestamped transaction proof. The worker validates CA TLS, channel/peer ledger convergence, active consenters and real chaincode write/query. Storage action checks actual PVC/PV EBS CSI claim bindings, not actual encryption/KMS/snapshot recovery. Isolation uses a positive worker authorization control before negative checks; it does not prove live Okta or CNI denial.

For an approved one-node disruption, authorize `disrupt` separately and pass `-node <approved-node-id>` plus the mutation flags above. Wait for readiness and use separately approved `verify` with a new idempotency key to run the same worker verification stages and produce a fresh transaction after recovery. This is an explicit mutating verification job, not a health GET. Never disrupt multiple Raft nodes simultaneously. Three-AZ placement, volume reattachment and actual AWS encryption must be independently checked and recorded by authorized administrators; the harness does not yet certify them.

For controller restart/adoption, capture `jobUID` while an operation is running. Stop/restart only the separate loopback qualification API using the original registry, approval and database; then run `-action adopt -operation $op -expected-job-uid <previous-uid> -allow-cloud-read`. UID stability is one check, not proof of every scheduler behavior. Dedicated PostgreSQL tests prove local persistence/fencing. AWS restart behavior is unverified until this procedure is executed.

For cancellation/retry, use separately approved `cancel` or `retry` with the same operation binding and mutation flags (retry additionally needs its own idempotency key). Observe durable history/status and Job termination. Use disposable fresh networks for failure scenarios. Do not induce failures in the retained user network or fabricate an AWS outage.

```powershell
./bin/fabric-acceptance.exe -action cleanup-preview @common -allow-cloud-read -operation $op -output <new-cleanup-preview-receipt.json>
# Inspect full administrator API preview, exact UIDs and retained data before approving.
# Only after a separate cleanup approval:
./bin/fabric-acceptance.exe -action cleanup @common -allow-cloud-read -execute -budget-authorization <approved-budget-id> -action-approvals <private-action-approvals.json> -operation $op -cleanup-digest <reviewed-preview-digest> -output <new-cleanup-result.json>
```

## Acceptance gates and limitations

No action sets `eksQualified=true`. Individual PASS checks are observations, not an aggregate release decision. Required additional evidence: actual AZ placement, authorized controlled disruptions with fresh successful transactions, EBS attachment/encryption and recovery, live tenant/Okta/CNI denial, controller containment, certificate lifecycle, consistent backup restoration, precise deletion and retained resource/cost reconciliation. The harness supports staged commands and explicitly leaves missing evidence NOT VERIFIED; it is not a fully automated enterprise acceptance suite. No AWS run has occurred.

Evidence contains controlled action names, hashes, timestamps, validated operation/Job/transaction IDs and fixed check IDs/statuses. It excludes raw API/AWS/Kubernetes responses, credentials, private keys and connection strings. Each output file must be new and is protected before writing. Ordinary unit tests use fake commands and make no cloud calls.
