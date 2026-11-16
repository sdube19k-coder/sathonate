# Manual setup and AWS qualification runbook

**Status:** Local Okta sign-in, the browser’s deployment authorization check,
AWS bootstrap, and selected AWS inventory reads passed. The UI integration and
teardown logic have fixture coverage. Live EKS deployment, IRSA, CNI isolation,
EBS recovery, and complete cloud teardown remain unqualified. The paid run is
paused and requires fresh approval. This procedure is for the existing
qualification account, with a USD 4 ceiling for an approved quick attempt.

## 1. Prepare the workstation

Work from the monorepo root, then use separate PowerShell tabs for the backend,
frontend, AWS operator and cleanup. Install Git, PowerShell 7, Docker Desktop
with Linux containers, Go and Node matching the repository manifests, Python 3,
AWS CLI v2 and kubectl. Check `docker info`, `go version`, `node --version`,
`npm --version`, `python --version`, `aws --version` and
`kubectl version --client`. Run `npm ci` under `fabric-console`. Keep the
retained kind cluster out of all provisioning and teardown commands.

Use [local development setup](local-kubernetes.md) for the separate kind
workflow and [deployment handoff](deployment-handoff.md) for environment
configuration. On a new workstation, provision a disposable local PostgreSQL
database with the repository schema and the development IAM-independent identity
registry. Use [PostgreSQL setup and migration notes](engineering-handbook/06-postgres.md)
and [Sprint 2 security setup](sprint2-production-security.md). The Okta and
Sprint 3 commands below assume the existing loopback `smp-postgres` container
and `fabric_smp_tests` database as documented in the console guide. The
Sprint 3 project tool also requires the already approved Sprint 1 viewer anchor.
A fresh database needs its own reviewed enrollment before using that tool.

## 2. Okta and the authorized local project

1. **Okta administrator.** In Applications, create or select the Sathonate
   **OIDC Single-Page Application**. Enable Authorization Code with PKCE. Set the
   sign-in URI to `http://127.0.0.1:5173/login/callback` and sign-out URI to
   `http://127.0.0.1:5173`. Assign the test user. Under Security → API, verify
   the custom authorization server issuer and audience, add `fabric.read` and
   `fabric.write` scopes, and allow those scopes and the SPA client in the
   access policy. Under the app's **Sign On → User authentication**, verify
   the applicable MFA rule. Inspect the user’s authenticator enrollment and
   sign-in rules if a passkey prompt blocks login. Keep MFA enforced in Okta.
2. **Local operator.** Start PostgreSQL and the API on **8081**, with
   `-mode local-development -local-oidc -deployment-mode disabled`. In the
   frontend tab run `npm run dev -- --mode development-okta` on **5173**.
   The [development Okta guide](../../fabric-console/docs/development-okta.md)
   gives the exact commands and `OKTA_ISSUER`, `OKTA_CLIENT_ID`,
   `OKTA_AUDIENCE` values. The test database password is read locally into the
   process environment. Keep it out of console output and Git.
3. **Local identity administrator.** Open the browser at
   `http://127.0.0.1:5173`, sign in, and obtain the verified issuer and subject
   from the authenticated project picker. Use the previously approved identity
   and the disposable test database. With `DATABASE_URL` set as above, run
   from `fabric-control-plane`:

   ```powershell
   go run ./cmd/fabric-sprint3-project -verified-issuer '<verified-issuer>' -verified-subject '<verified-subject>'
   ```

   The command saves or checks the exact
   [qualification design](../examples/sprint3-protected-eks-network.json),
   grants only `read` and `deploy` to the disposable project for four hours,
   and prints the actual network ID, storage scope and typed spec digest for
   the runtime binding. It does not elevate the existing viewer project.
4. **Browser user.** Sign in again after a grant or renewal so the token is
   issued after the grant starts. Select **disposable-eks → Networks →
   Sprint 3 protected EKS qualification**. Select **Authorize deployment with
   Okta** to request `fabric.write`, then **Verify deployment access**. Look
   for the server confirmation for the saved revision. This check is read-only
   and works in History mode. A 404 for the authorization endpoint can mean the
   local API is still running an older executable. Rebuild and restart it in
   disabled mode, then retry. Keep unrelated project grants unchanged.

## 3. AWS account and IAM assignments

The scripts are deliberately bound to account `429488690613`, region
`ap-south-1` and stack/EKS/ECR name `sathonate-s3-20261027`. Changing account
or reusing the workflow for another customer requires review of the policies,
fixed runner checks and template together.

| Identity Center permission set | Local CLI profile | Role during the run |
| --- | --- | --- |
| `Sprint3-permissions` | `sathonate-sprint3` | CloudFormation, EKS, exact ECR repository, inventory and teardown |
| `Sprint3-Bootstrap` | `sathonate-bootstrap` | Create exact tagged roles and empty repository, bind the exact EKS issuer, detach/delete run IAM identities at cleanup |

1. **AWS account administrator.** Open IAM Identity Center in Mumbai.
   Under **Multi-account permissions → Permission sets → Create permission
   set**, choose **Custom permission set → Inline policy**. Create or update
   the two sets using the exact current policy files:
   [deployer JSON](../tools/sprint3/iam/sprint3-permissions-policy.json) and
   [bootstrap, issuer and cleanup JSON](../tools/sprint3/iam/sprint3-bootstrap-with-issuer-policy.json).
   The bootstrap-only JSON is for the initial phase and lacks issuer binding
   and IAM teardown actions. Wait for account provisioning to succeed after
   every update. No broad admin policy is required for the deployment roles.
2. Under **AWS accounts → Sushil-19 → Assign users or groups**, assign both
   permission sets to the approved Identity Center identity. Use the owner
   console only for permission-set administration where necessary. The scripts
   require assumed roles for execution.
3. **Operator, new PowerShell tab.** Run `aws configure sso --profile
   sathonate-sprint3`, select `Sprint3-permissions`, then repeat with
   `sathonate-bootstrap` and select `Sprint3-Bootstrap`. Use the access portal
   `https://d-9f675bb449.awsapps.com/start`, SSO and default client region
   `ap-south-1`, and default scope `sso:account:access`. Select account
   `429488690613`. Decline the optional skills and MCP install. Run:

   ```powershell
   aws sso login --profile sathonate-sprint3
   aws sso login --profile sathonate-bootstrap
   aws sts get-caller-identity --profile sathonate-sprint3 --region ap-south-1
   aws sts get-caller-identity --profile sathonate-bootstrap --region ap-south-1
   ```

   Verify the account and respective `AWSReservedSSO_` role families. Use the
   persistent IAM role ARN for the EKS access entry. STS prints the session ARN.
   Refresh both sessions near the run and keep access available through cleanup.

## 4. Bootstrap the service roles and registry

In the operator tab under `fabric-control-plane`:

```powershell
.\tools\sprint3\iam\admin-bootstrap.ps1 -AdminProfile sathonate-bootstrap -Action Plan
.\tools\sprint3\iam\admin-bootstrap.ps1 -AdminProfile sathonate-bootstrap -Action Bootstrap
```

Run **Bootstrap** only after approval for the IAM/ECR setup. It checks existing
ownership and settings, creates missing standard service-linked roles, and
creates or verifies the run-tagged immutable AES256 private repository. It
creates these four run-tagged roles:

| Role suffix | Trust | Managed policies |
| --- | --- | --- |
| `-clusterrole` | `eks.amazonaws.com` | `AmazonEKSClusterPolicy` |
| `-noderole` | `ec2.amazonaws.com` | `AmazonEKSWorkerNodePolicy`, `AmazonEC2ContainerRegistryPullOnly` |
| `-cnirole` | Deny-only until IRSA binding, then exact `kube-system:aws-node` subject | `AmazonEKS_CNI_Policy` |
| `-ebsrole` | Deny-only until IRSA binding, then exact `kube-system:ebs-csi-controller-sa` subject | `service-role/AmazonEBSCSIDriverPolicy` |

The [trust templates and IAM rationale](../tools/sprint3/iam/sprint3-admin-bootstrap.md)
describe exact subjects and conditions. OIDC provider creation uses the cluster's
actual issuer and audience `sts.amazonaws.com`. An account-level Okta issuer
and an EKS service-account issuer serve separate purposes. Never substitute one
for the other.

## 5. Read-only preflight and paid launch

1. Verify the stack is absent, no prior run resources exist, and the run
   repository has the correct ownership tag and is empty. Check EC2 Standard
   On-Demand vCPU usage and quota against six required vCPUs. Check EKS standard
   support for the pinned 1.34 version and availability of the instance type
   in all three zones. Confirm the default Mumbai EBS key and review any
   existing customer-managed key charges separately. Use AWS Billing → Bills
   and Credits to confirm current liability and credit coverage. Price EKS,
   EC2, EBS, IPv4, logs, image storage, data transfer and tax, including
   cleanup. Historical estimates and credit readings are dated evidence.
2. Stage the administrator-owned template, the verified Linux/amd64 image
   manifest inventory, exact operator public IPv4 /32, and pinned Bevel CRDs
   from commit `cd236ccc61d1d159b3dadb4625d08bb5682a8451`. Use a new
   private output directory outside Git. Never apply the kind bootstrap to EKS.
   Confirm effective scope and current credentials for stack, ECR and IAM
   teardown. The deployer was denied IAM simulation. This denied read alone
   provides no evidence about the actual delete actions.
3. **Get fresh approval before paid activity**, including pushing images.
   Arm the independent cleanup terminal before the first paid command using
   the exact kubeconfig path, scope, network, issuer journal, signal and
   deadline described in the [launch checklist](sprint3-launch-checklist.md).
4. Push the qualified worker, controller, smoke chaincode and Fabric peer,
   orderer and CA images to the run ECR. Obtain ECR credentials with
   `aws ecr get-login-password` piped to `docker login --password-stdin`.
   Record ECR manifest digests via `aws ecr describe-images`. Compare the
   controller manifest to the [pinned source digest](../internal/deployment/secure_profile.go).
   Abort and clean up if it differs.
5. Generate `infrastructure-admin-owned.json` with
   `python tools/sprint3/prepare_admin_owned.py --output <new-directory>`.
   Use `aws cloudformation validate-template` first. Create the exact named
   stack using the deployer profile. Set `OperatorRoleArn` and
   `OperatorCIDR`, supply `SathonateRun=sathonate-s3-20261027` on the stack,
   and leave `EnableWorkloads=false`. Use `describe-stacks` and
   `describe-stack-events` to record success or the exact failure.
6. Record the cluster issuer from the exact stack output or
   `aws eks describe-cluster`. Run
   `.\tools\sprint3\iam\admin-bootstrap.ps1 -AdminProfile sathonate-bootstrap -Action BindIssuer -Issuer $issuer`.
   Update that same stack with all original parameters and tags, changing
   only `EnableWorkloads` to `true`. Verify three Ready workers, the strict
   VPC CNI node agent, EBS CSI and the verified cluster endpoint.
7. Run [write-eks-kubeconfig.ps1](../scripts/write-eks-kubeconfig.ps1) with
   the deployer profile, Mumbai, exact run name as cluster and context, account,
   actual IAM role ARN and reserved absolute output path. Use that file and
   context on every EKS Kubernetes command. Apply the pinned source CRDs and
   [encrypted gp3 StorageClass](../tools/sprint3/storage-class.yaml). Assemble
   the [secure runner binding](sprint3-local-secure-eks.md#trusted-api-binding)
   from the saved project output, actual registry digests and kubeconfig.
   Specify encrypted storage, 10 GiB, brokered channels and per-node zone
   selectors matching the saved design. Use an expiring approval reference.
   Restart only the loopback API with the secure EKS mode and binding.
8. **Use the browser to deploy.** Open the saved revision in `disposable-eks`.
   Verify access and click **Deploy approved EKS network**. Record the
   operation ID, stages, component health, transaction ID and readback
   evidence. The dashboard shows the committed verification recorded during
   provisioning. A separate dashboard-driven transaction is currently
   outside this path. Record unexecuted gates explicitly.

### Operator command sequence

Run in the operator PowerShell tab from `fabric-control-plane` after completing
the preceding gates. Replace the three marked inputs and keep the work
directory outside the checkout. Keep the cleanup terminal armed before step 4
above. The following sequence is for one approved attempt.

```powershell
$run = 'sathonate-s3-20261027'
$region = 'ap-south-1'
$profile = 'sathonate-sprint3'
$roleArn = '<exact provisioned IAM role ARN>'
$operatorCidr = '<verified public IPv4>/32'
$work = '<new absolute output directory>'
$kubeconfig = Join-Path $work 'eks.json'
$stackArgs = @('--stack-name', $run, '--region', $region, '--profile', $profile, '--no-cli-pager')
python tools/sprint3/prepare_admin_owned.py --output $work
if ($LASTEXITCODE -ne 0) { throw 'Template generation failed' }
$template = 'file://' + (Join-Path $work 'infrastructure-admin-owned.json')
aws cloudformation validate-template --template-body $template --region $region --profile $profile
if ($LASTEXITCODE -ne 0) { throw 'Template validation failed' }

# Only after approval and arming cleanup
$parameters = @("ParameterKey=OperatorRoleArn,ParameterValue=$roleArn",
  "ParameterKey=OperatorCIDR,ParameterValue=$operatorCidr")
aws cloudformation create-stack @stackArgs --template-body $template --tags "Key=SathonateRun,Value=$run" --parameters @parameters 'ParameterKey=EnableWorkloads,ParameterValue=false'
if ($LASTEXITCODE -ne 0) { throw 'Stack submission failed. Check exact stack status, then trigger cleanup.' }
aws cloudformation wait stack-create-complete @stackArgs
if ($LASTEXITCODE -ne 0) { throw 'Stack creation unverified. Trigger cleanup.' }
$issuer = aws cloudformation describe-stacks @stackArgs --query "Stacks[0].Outputs[?OutputKey=='OIDCIssuer'].OutputValue | [0]" --output text
if ($LASTEXITCODE -ne 0 -or $issuer -notmatch '^https://oidc\.eks\.ap-south-1\.amazonaws\.com/id/[a-fA-F0-9]+$') { throw 'Issuer missing. Trigger cleanup.' }
.\tools\sprint3\iam\admin-bootstrap.ps1 -AdminProfile sathonate-bootstrap -Action BindIssuer -Issuer $issuer
if ($LASTEXITCODE -ne 0) { throw 'Issuer binding failed. Trigger cleanup.' }
aws cloudformation update-stack @stackArgs --use-previous-template --tags "Key=SathonateRun,Value=$run" --parameters @parameters 'ParameterKey=EnableWorkloads,ParameterValue=true'
if ($LASTEXITCODE -ne 0) { throw 'Stack update failed. Trigger cleanup.' }
aws cloudformation wait stack-update-complete @stackArgs
if ($LASTEXITCODE -ne 0) { throw 'Workload infrastructure unverified. Trigger cleanup.' }
.\scripts\write-eks-kubeconfig.ps1 -AwsProfile $profile -Region $region -Cluster $run -Context $run -AccountID '429488690613' -RoleARN $roleArn -Output $kubeconfig
kubectl --kubeconfig $kubeconfig --context $run get nodes -L topology.kubernetes.io/zone
kubectl --kubeconfig $kubeconfig --context $run apply -f '<pinned Bevel checkout>/config/crd/bases'
kubectl --kubeconfig $kubeconfig --context $run apply -f tools/sprint3/storage-class.yaml
```

The cleanup deadline takes priority over the CLI waiters. Stop the operator
workflow when the deadline or cleanup signal is reached. Keep the issuer
journal, evidence and cleanup terminal available. If any prerequisite step
fails, trigger cleanup immediately and inspect the exact stack events rather
than continuing with the remaining commands.

## 6. Cleanup and handoff

1. Signal the independent helper immediately after recording success or
   failure. The helper fences the API runner and UID-checks the run
   namespace. It waits for PVC-backed volumes while CSI is available and
   can fall back to bounded stack deletion and exact run-tagged, detached
   volume removal. Use the same issuer journal if retrying.
2. Verify the exact stack, EKS cluster, instances, volumes, ENIs, VPC, NAT,
   IPv4 and load balancer inventory. Verify the captured backing volume IDs
   and exact log group independently. Escalate failed or timed-out cleanup
   immediately to the AWS administrator with the owned IDs.
3. Verify ECR removal and absence of the four run-owned roles and the exact
   issuer provider. The bootstrap helper checks the repository tag and invokes
   deletion with the deployer role. Revoke the temporary Identity Center
   assignment only after verifying removal. Preserve shared service-linked
   roles, unrelated KMS keys and the retained local network.
4. Return the local API to History mode, let the expiring test grant lapse
   or have the local identity administrator revoke it, and remove temporary
   local cloud config and registry authentication when no longer needed.
   Review AWS Billing after metering catches up. Record result and cleanup
   separately in redacted evidence.

## Related manual gates

- [Console walkthrough](../../fabric-console/docs/console-walkthrough.md)
  covers saved drafts, deployment stages and read-only history.
- [RC1 readiness](rc1-readiness.md) tracks production Okta, customer-owned
  Kubernetes, live payments, multi-node and disaster recovery. Production
  acceptance and launch require their own credentials, review and approvals.
  Keep payments and production provisioning disabled for this run.
- [Sprint 3 preflight and evidence](evidence/sprint3-quick-check-20261112.txt)
  distinguishes earlier observations from fixture tests and unexecuted work.

Provider references: [IAM Identity Center permission sets](https://docs.aws.amazon.com/singlesignon/latest/userguide/howtocreatepermissionset.html),
[AWS CLI SSO](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-sso.html),
and [EKS issuer setup](https://docs.aws.amazon.com/eks/latest/userguide/enable-iam-roles-for-service-accounts.html).
