# Fabric SMP cloud launch setup

Current state: the real local kind/Bevel/Fabric workflow works. The company AWS/EKS environment and Okta application still need to be created. Cloud resource creation and production execution are disabled. A separately approved EKS qualification adapter is implemented; see [EKS worker](eks-worker.md). The Cloud readiness screen reports configuration separately from read-only access evidence and from production qualification; it never converts a checklist into deployment success.

## What the account owners must supply

1. A company-owned AWS account with billing, MFA and an administrator who can configure IAM Identity Center/SSO. Choose the deployment region and a monthly budget. Do not send root credentials or access keys to the application or chat.
2. Approval for an EKS/VPC/node/storage design and its cost. Decide public/private API access, how the operator reaches the API, availability zones, private nodes, scoped workload roles, encrypted EBS storage, DNS/TLS and image registry. An EKS cluster and worker nodes are paid resources; no creation command has been run here.
3. A company Okta organization and an administrator who can create a SPA application and custom authorization server. Confirm authorization-server entitlement for the intended production subscription.
4. Company-controlled console and API domains, DNS administration and certificates. Prefer the console and `/api` behind one HTTPS origin where practical.

After account creation, provide only the AWS account ID, AWS profile name, region, EKS cluster name/context, owned domain names, Okta issuer, SPA client ID and API audience. These are configuration identifiers. Keep actual credentials local or in the approved secret system.

## AWS access setup

Install AWS CLI v2 and configure company SSO. These steps configure access and read existing resources; they do not create a cluster:

```powershell
aws configure sso --profile sathonate-admin
aws sso login --profile sathonate-admin
aws sts get-caller-identity --profile sathonate-admin
# After an approved EKS cluster has actually been created:
aws eks update-kubeconfig --profile sathonate-admin --region <chosen-region> --name <cluster-name> --alias sathonate-eks
kubectl --context sathonate-eks get nodes
```

Use an explicit profile and context. Never rename the cloud context to `kind-smp-qualification`, and never point the local runner at EKS. The application checks the AWS account and EKS endpoint binding before probing cluster readiness. Wrong bindings stop the checks.

Before approving cluster creation, prepare and review a provider configuration and cost estimate. The official eksctl dry-run command produces configuration without cluster creation:

```powershell
eksctl create cluster --config-file <reviewed-cluster-config.yaml> --dry-run
```

Do not remove `--dry-run` until creation is explicitly authorized. EBS-backed Fabric volumes require the EBS CSI driver, suitable IAM permissions and an encrypted `ebs.csi.aws.com` StorageClass with `WaitForFirstConsumer`. Passing a StorageClass read check is not a PVC attachment/recovery test. See [AWS EKS cluster setup](https://docs.aws.amazon.com/eks/latest/userguide/create-cluster.html), [eksctl dry run](https://docs.aws.amazon.com/eks/latest/eksctl/dry-run.html), and [EBS CSI requirements](https://docs.aws.amazon.com/eks/latest/userguide/ebs-csi.html).

## Okta configuration

Create an OIDC **Single-Page Application**, with authorization code and PKCE, without a client secret in the SPA. Configure these URIs for local qualification:

- Sign-in redirect: `http://127.0.0.1:5173/login/callback`
- Sign-out redirect / trusted origin: `http://127.0.0.1:5173`
- Production: the equivalent company-owned HTTPS console origin and `/login/callback`.

Create a custom authorization server issuer such as `https://<company>.okta.com/oauth2/<server-id>`, a distinct API audience such as `api://fabric-smp`, scopes `fabric.read` and `fabric.write`, and trusted access-token tenant/role claims. Map controlled groups `sathonate-admin`, `sathonate-operator`, and `sathonate-viewer`. The default tenant claim is `sathonate_tenant`. The Go API validates access tokens, issuer, audience, SPA client binding, scopes and claims; it does not accept ID tokens as API credentials. Test positive and negative tenant/role cases with real company users before production. See [Okta SPA redirect guidance](https://developer.okta.com/docs/guides/sign-into-spa-redirect/react/main/).

## Non-secret server environment

Use the console's **Cloud readiness → Download Configuration Template**, or set these values in the terminal that starts the API. The template contains identifiers only, not credentials. Do not place AWS access keys, bearer tokens, client secrets or kubeconfig contents in it.

```powershell
$env:CLOUD_AWS_ACCOUNT_ID='<12-digit-account-id>'
$env:CLOUD_AWS_PROFILE='sathonate-admin'
$env:CLOUD_AWS_REGION='<chosen-region>'
$env:CLOUD_EKS_CLUSTER='<existing-approved-cluster>'
$env:CLOUD_KUBE_CONTEXT='sathonate-eks'
$env:CLOUD_STORAGE_CLASS='<encrypted-ebs-storage-class>'
$env:CONSOLE_PUBLIC_URL='https://<console-domain>'
$env:API_PUBLIC_URL='https://<api-domain>'
$env:OKTA_ISSUER='https://<company-okta-domain>/oauth2/<server-id>'
$env:OKTA_CLIENT_ID='<spa-client-id>'
$env:OKTA_AUDIENCE='api://fabric-smp'
$env:OKTA_TENANT_CLAIM='sathonate_tenant'
```

Restart the local API after configuration, then open Cloud readiness and run the read-only checks. `GET /api/v1/readiness` inspects configuration; `POST /api/v1/readiness/probe` checks explicit AWS identity, active EKS cluster, matching Kubernetes API endpoint, API readiness and encrypted EBS StorageClass. Without complete explicit access configuration, no cloud command runs. The local setup endpoints are not exposed to enterprise tenants in production mode.

## What is still a production release gate

The local execution runner deliberately rejects nonlocal targets. The EKS qualification adapter now reuses the pinned operator workflow with scoped tenant workers, a channel broker, digest-bound administrator approval, storage/placement translation and PostgreSQL recovery. Operator targets bypass the old non-executable Ansible/Flux exporter. Production execution still requires actual EKS qualification, live Okta login, backup restoration, certificate rotation and security review. Supplying credentials or setting a flag cannot bypass these gates. See [the EKS worker setup and test commands](eks-worker.md).

Acceptance must prove actual Fabric CA enrollment, peers, three Raft orderers, channel membership, chaincode endorsement/commit/query from both organizations, failure recovery and tenant isolation on the approved EKS environment. Until those checks pass, the UI must keep `executionEnabled=false` and show the blockers. The AWS account, Okta application and paid-resource authorization cannot be fabricated by local tests.
