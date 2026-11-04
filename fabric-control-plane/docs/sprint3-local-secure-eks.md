# Sprint 3 secure EKS adapter — local qualification

## Tracking update — 30 October 2026

**Sprint 3: LOCAL QUALIFICATION PASSED / AWS EKS QUALIFICATION PENDING.**
Sprint 3 remains open. AWS execution is explicitly deferred; no AWS resources,
permissions or billing APIs are used for this continuation. The 30 October
disposable API/Fabric and Calico qualification passed with new transactions,
peer/PVC recovery and current retained-network read-only readiness of 8/8.
See [Sprint 4 local work](sprint4-local-platform.md) for the current local scope
and final evidence references. The October 27 results below retain their original
date and limits; they are not substituted for current health checks.

Preserve the EKS adapter, IAM/ECR preparation, six-scenario atomic evidence and
all unexecuted live EKS/IRSA/VPC-CNI/EBS acceptance gates. Real Okta and historical
private-key remediation remain open. Local PASS does not close these gates or
authorize AWS deployment or a production release.

2026-10-27. AWS deployment is **BLOCKED**. No AWS resources, IAM policies, plan settings or account permissions were changed during this work. SSO and quota checks were not repeated.

## Implementation

`internal/deployment/secure_eks.go` implements a separate `secure-eks` Runner for the reserved `smp-secure-eks-v1` target. It reuses the protected worker/controller, manifest transformation and isolation code. The original kind-only constructor remains restricted to disposable kind and loopback Kubernetes. Legacy workers reject both protected target IDs; there is no original-controller fallback.

The trusted binding fixes account 429488690613, Mumbai, run `sathonate-s3-20261027`, the `sathonate-sprint3` profile, the `Sprint3-permissions` assumed-role family, one authenticated PostgreSQL storage scope and one saved network ID/design hash. It deep-copies runtime configuration, pins the worker and controller images to the run's private ECR repository, pins the kubeconfig bytes, and rejects embedded credentials, proxy settings and alternate credential commands. The controller image must preserve the already qualified manifest digest. ECR publishing/digest preservation is not yet verified.

The saved desired design remains AWS. A deep-copied worker input is narrowed to the exact previously qualified two-org/two-peer/three-orderer/channel profile. AWS node placement must agree with administrator-approved runtime selectors, with three distinct orderer AZs. The worker still uses its protected Secret-input protocol. Private material is never added to a public design/CR/Helm value or process argument.

Before workload mutation the adapter checks signed AWS identity, exact EKS ARN/endpoint/CA, strict AWS exec kubeconfig identity, Kubernetes minor, three ready AZs, encrypted gp3 delayed-binding StorageClass, CSI registration, strict ready VPC CNI/node-agent configuration and required CRDs. Actual AWS CNI enforcement, IAM/IRSA behavior, EBS attachment/encryption and recovery still require live qualification; metadata/unit fixtures are not that proof.

The API uses the existing PostgreSQL durable submission, idempotency, cancellation, Job adoption and fencing. Recovery will not replay a different runner's records or another network's binding. `secure-eks` startup requires explicit local-development, loopback binding, configured OIDC and server-owned project memberships; implicit developer identity and local bearer bypass are rejected. Existing Okta validation is unchanged. Initial real-user read-only memberships are not escalated automatically.

Destructive API cleanup requires administrator authorization and a fresh preview digest. It checks namespace ownership/UID and deletes only exact UID-bound workload references. Namespace, Secrets, PVCs and AWS infrastructure are excluded; operator cascading retention is not assumed. Separate verified PVC/cloud teardown remains necessary. An expired qualification config cannot start deployments; it can still be loaded for scoped diagnosis/cancellation/approved cleanup. A future authorization window greater than four hours is rejected. IAM/session duration is not a hard spending or resource-runtime limit.

## Local results and limits

| Gate | Result |
| --- | --- |
| New adapter configuration, scope, root/endpoint/kubeconfig drift, unsafe storage/CNI/AZ, protected manifest and stale cleanup approval tests | PASS, local command fixtures; no AWS execution |
| Full Go suite | PASS: 252 tests/subtests, 16 explicit skips |
| Go vet | PASS |
| Real PostgreSQL authorization/durability/recovery regressions | PASS: 39 tests/subtests, one explicit TLS opt-in skip; includes secure-EKS idempotent recovery and refusing foreign-scope adoption |
| Explicit PostgreSQL verified-TLS/restricted-role Docker fixture | PASS, run separately with its opt-in enabled |
| Authenticated API → real disposable Fabric lifecycle | PASS: current protected worker build, two CAs/two peers/three orderers, channel/chaincode/committed transactions and API restart/retry |
| Credential/private-key and effective Calico isolation inspection | PASS: public CRs, Helm history, sampled arguments/logs, cross-namespace ingress/egress, foreign-resource denial and live peer certificates |
| Infrastructure preparation | PASS: nine offline tests; AWS CloudFormation service acceptance NOT EXECUTED |
| Python safety/evidence regressions | PASS: 32 tests |
| Console unit/build | PASS: 16 tests and build; console → EKS not executed |
| Startup security gates | PASS: six negative binary invocations |
| IAM static policy validation | PASS for actual `Sprint3-permissions` policy; live policy simulator not executed |
| Administrator command preparation | PowerShell parse/offline Plan PASS; ten AWS CLI command schemas recognized locally; mutations NOT EXECUTED |
| Live EKS integration/IRSA/CNI/EBS recovery | NOT EXECUTED |

The disposable runtime run exercises the reused protected API/Fabric path on kind, **not an EKS cluster or mocked-live-Okta acceptance**. New EKS control-plane checks and manifest portability are covered by local fixtures. The controlled HTTPS OIDC fixture is distinct from real Okta. The retained network was observed read-only at 8/8 ready; the new disposable cluster was removed. Earlier six-scenario atomic recovery evidence is preserved, not relabeled as a fresh run or qualification of every Fabric identity class.

Redacted committed evidence: `docs/evidence/sprint3-local-secure-eks.json`. Verbose local logs: `.logs/sprint3-adapter-go.jsonl`, `sprint3-adapter-postgres.jsonl`, `sprint3-qualification.json`, `sprint3-api-kind.json`, `sprint3-worker-build.json`, `sprint3-python-regressions.log`, `sprint3-startup-denials.json`. Console logs are in the console repository's `.logs/sprint3-adapter-*` files.

## Administrator actions

Use [administrator commands](../tools/sprint3/iam/admin-commands.md) and the non-root, default-Plan [bootstrap helper](../tools/sprint3/iam/admin-bootstrap.ps1). Bootstrap the four exact tagged IAM roles and private ECR repository without EKS/EC2 creation. CNI/EBS roles initially cannot be assumed. Their exact issuer/audience/service-account trust is bound only after a separately authorized cluster exists; coordinate administrator availability before starting that chargeable phase. Preserve shared service-linked roles.

The actual permission set is `Sprint3-permissions`; use `tools/sprint3/iam/sprint3-permissions-policy.json`, not the earlier permission-set-name draft. The deployer retains only GetRole/PassRole on the four approved roles; no IAM role creation/editing is added.

After billing/admin prerequisites and live dependencies are resolved, generate the two-phase infrastructure with `python tools/sprint3/prepare_admin_owned.py --output <new-directory>`. This generator makes no AWS call. Do not use the original in-stack-IAM template directly.

## Trusted API binding

Build `go build -o bin/fabric-api-sprint3.exe ./cmd/fabric-api`. The administrator-owned `SecureEKSConfig` JSON schema is defined by that type in `internal/deployment/secure_eks.go`; the loader rejects unknown fields and trailing JSON. Supply the actual immutable ECR image digests and exact AWS-exec JSON kubeconfig produced by `scripts/write-eks-kubeconfig.ps1`, with context `sathonate-s3-20261027`. Do not invent cluster CA, issuer, tenant/project IDs or deployment hashes.

`tenantId` is the server-resolved `Principal.StorageTenant()` key (the `lp-...` tenant/project isolation key), not a browser-provided tenant UUID. `networkId` is the saved PostgreSQL record ID. `networkSHA256` is lowercase SHA256 of `json.Marshal(savedDesiredNetworkSpec)` using the Go typed domain contract. Runtime must specify `smp-gp3-encrypted`, `10Gi`, the approved tagged/digest-pinned Fabric images and per-node AZ selectors. Bind an authorization reference and original expiry no more than four hours ahead. Keep that binding unchanged for recovery.

The future loopback qualification invocation is:

```powershell
./bin/fabric-api-sprint3.exe -mode local-development -local-oidc -deployment-mode secure-eks -secure-eks-config <administrator-owned-binding.json>
```

The usual environment-only PostgreSQL connection and supported Okta issuer/client/audience configuration are required. No token/password/private key belongs in command arguments. Do not add `-enable-cloud-execution`, `-local-token-grants` or legacy `-cloud-config` as a workaround. Real authentication/membership write grants and runtime acceptance remain open prerequisites. No membership changes were performed here.

## Billing findings and deployment decision

The earlier SSO-authenticated Free Tier API observation was **PAID / ACTIVE / USD 0 plan credits**. That is a Free Tier plan-credit balance, not proof that all promotional credits are absent. This continuation made one read-only `billing:GetCredits` request and one `organizations:DescribeOrganization` request; both returned AccessDenied. Promotional balance, eligibility and the exact plan-change cause therefore remain **BLOCKED / UNKNOWN**.

AWS documents that joining Organizations or setting up Control Tower during an upgrade immediately expires Free Tier credits. This could explain the change around Identity Center setup, but organization membership/change history was not verified. Do not claim this as the actual cause. See the [Free Tier FAQ](https://docs.aws.amazon.com/awsaccountbilling/latest/aboutv2/free-tier-FAQ.html) and [GetCredits reference](https://docs.aws.amazon.com/aws-cost-management/latest/APIReference/API_billing_GetCredits.html). The administrator commands include the exact read-only queries and console checks for active balance, expiry and applicable services.

Using previously retrieved Mumbai prices, three m7i-flex.large workers, standard EKS, three public IPv4 addresses and 160GiB gp3 total about **$1.75 for four hours**, excluding image/log/traffic extras. This is an estimate, not a refreshed quote or verified credit coverage. A $5 ancillary reserve still fits the planned $20, but paid-plan usage can be billed to the payment method and failed teardown can exceed any estimate. Nothing was deployed; no new AWS infrastructure usage was initiated by this work.

Deployment is **NOT READY** until billing is verified, administrator dependencies exist, exact image/config provenance is bound, real API authentication/write membership is accepted, the approved target's preflight passes and reliable full teardown is ready. Live EKS qualification remains unexecuted. Sprint 2 real Okta and historical-key remediation gates stay open; production remains disabled. Sprint 4 planning may proceed, but Sprint 3 is not closed.
