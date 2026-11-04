# Sathonate Sprint 3 permission set

**Current assignment:** use `sprint3-permissions-policy.json` for the actual `Sprint3-permissions` permission set, with `sprint3-admin-bootstrap.md` and `admin-commands.md`. The older `sprint3-identity-center-policy.json`, `SathonateSprint3Deployer.inline.json` and historical validation records refer to the superseded permission-set name; do not paste those drafts into the current assignment.

Prepared 2026-10-27. No AWS API calls, IAM changes or resource creation were performed for this policy task. Public AWS documentation/service-reference downloads were used. Policy syntax, action names and size were checked locally; live IAM simulation, CloudFormation validation and deployment remain NOT EXECUTED.

## Scope and existing implementation

Attach `SathonateSprint3Deployer.inline.json` to permission set `SathonateSprint3Deployer`, account `429488690613`, region `ap-south-1`. The one reserved run name is `sathonate-s3-20261027`; this name is a proposed binding, not an existing resource. Do not replace it with a broad wildcard. Regenerate/review for a different run.

Inspected `tools/sprint3/prepare.py`, `scripts/write-eks-kubeconfig.ps1`, `internal/deployment/eks.go`, `docs/eks-qualification.md` and `docs/eks-implementation-map.md`. No Terraform or eksctl provisioning files were found in the Sathonate workspace search. The current infrastructure provisioner generates CloudFormation. The secure application-to-EKS adapter is still a separate blocker; this policy does not implement or qualify it.

The human permission set uses CloudFormation's caller permissions, not a service execution role. It permits creation/inspection/deletion of the exact named stack, protected EKS provisioning, tagged VPC resources, image upload to one administrator-created repository and scoped cleanup. It deliberately omits stack updates/change sets, IAM creation/editing/deletion, role assumption, subscriptions, plan changes, quota changes, NAT/EIP/LB creation, direct instance launches, Fargate and Auto Mode. Do not attach broader managed policies alongside it. Do not supply `--role-arn` to CloudFormation. `write-eks-kubeconfig.ps1`'s `RoleARN` parameter is the actual Identity Center IAM role ARN, not a role to assume.

## Administrator prerequisites (not performed)

An administrator must create exactly these IAM roles, applying the organization's approved permission boundaries. No permissions to create, edit trust, attach policies or delete these roles are granted to the deployer:

| Role name | Trust | AWS managed policy ARN |
| --- | --- | --- |
| `sathonate-s3-20261027-clusterrole` | `eks.amazonaws.com`, `sts:AssumeRole` | `arn:aws:iam::aws:policy/AmazonEKSClusterPolicy` |
| `sathonate-s3-20261027-noderole` | `ec2.amazonaws.com`, `sts:AssumeRole` | `arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy` and `arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryPullOnly` |
| `sathonate-s3-20261027-cnirole` | Exact cluster OIDC provider; conditions below | `arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy` |
| `sathonate-s3-20261027-ebsrole` | Exact cluster OIDC provider; conditions below | `arn:aws:iam::aws:policy/service-role/AmazonEBSCSIDriverPolicy` |

All role ARNs are listed in `approved-roles.json`. `iam:GetRole` is allowed only on those four roles. `iam:PassRole` is restricted to their exact ARNs and relevant EKS/EC2 service principals. The deployer cannot pass any CloudFormation execution role or Identity Center role.

The administrator must also ensure the standard service-linked roles exist: `AWSServiceRoleForAmazonEKS` (service `eks.amazonaws.com`), `AWSServiceRoleForAmazonEKSNodegroup` (`eks-nodegroup.amazonaws.com`), and `AWSServiceRoleForAutoScaling` (`autoscaling.amazonaws.com`). Their AWS service-linked policies are service-managed; no human role administration is granted here. Remove only newly created run-exclusive dependencies after qualification; do not delete shared service-linked roles.

IRSA requires a staged administrator step after EKS reports its real issuer. Create the IAM OIDC provider for that exact HTTPS issuer, client ID `sts.amazonaws.com`, using validated provider TLS trust. For CNI/EBS roles use `sts:AssumeRoleWithWebIdentity` with the exact provider ARN as `Principal.Federated`, and BOTH `StringEquals` conditions:

- `<issuer-without-https>:aud` = `sts.amazonaws.com`.
- `<issuer-without-https>:sub` = `system:serviceaccount:kube-system:aws-node` for CNI, or `system:serviceaccount:kube-system:ebs-csi-controller-sa` for EBS.

Do not wildcard issuer, audience or subject. CNI/EBS roles may be created beforehand with no federated access, then have trust set by the administrator once the issuer is known. Record and later remove the new run-specific OIDC provider and four roles. This staged admin task must be completed before starting addons/node provisioning, not while leaving a billable cluster waiting indefinitely.

Administrator: create private ECR repository `sathonate-s3-20261027` in Mumbai, immutable image tags and encryption enabled, restricted to this account. Only digest-verified qualified images may be uploaded. The deployer can empty/delete this exact repository during teardown; it cannot create repositories or alter repository policies. The node role pulls using its approved ECR policy.

## Required template changes before use

The current generated template is intentionally incompatible with these restricted permissions: it contains `AWS::IAM::Role` and `AWS::IAM::OIDCProvider`. Do NOT solve the resulting denial by adding IAM creation privileges.

Before deployment, review a two-phase template/runbook that:

1. Removes the four IAM role resources and OIDC provider resource. References the four exact administrator-owned role ARNs instead. Phase one creates VPC/logging/cluster/access; phase two creates addons/nodes after administrator IRSA setup. This file does not modify the existing generator.
2. Sets `ClusterName` and CloudFormation stack name to `sathonate-s3-20261027`, `Mode=FullFabric`, and tags the stack and all new supported resources `SathonateRun=sathonate-s3-20261027`. Keep EKS Kubernetes `1.34`, `upgradePolicy.supportType=STANDARD`, API authentication, creator bootstrap admin disabled and self-managed addons disabled. Check standard-support availability again before execution; change version only through a reviewed policy/template update.
3. Sets `OperatorRoleArn` to the actual IAM role ARN provisioned for permission set `SathonateSprint3Deployer` in account 0613. Never use the STS session ARN. Access creation is limited to this permission-set role name family; access policy association is limited to `AmazonEKSClusterAdminPolicy` on this run's access entry. That is Kubernetes administration of this disposable cluster, not AWS AdministratorAccess. Keep one assignment active to preserve the Identity Center role ARN.
4. Keeps three fixed Mumbai AZ nodegroups `zone-a`, `zone-b`, `zone-c`, approved instance types, encrypted gp3, IMDSv2, CNI policy enforcement and endpoint operator /32. Tag EBS claims through EBS CSI with the same run tag for scoped orphan cleanup. Restrict workloads through Kubernetes RBAC and network policy separately: IAM does not authorize tenant/project API requests.
5. Deletes PVCs while CSI is operational, then workloads/addons/nodegroups/cluster/VPC stack/repository. Verify all EC2/EBS/ENI/NAT/EIP/LB inventories. Human orphan deletion is allowed ONLY when the exact `SathonateRun` tag already exists. EKS-managed/shared or untagged orphans must go through verified owner/admin recovery; the deployer cannot retag an unrelated resource to gain deletion access. Admin-owned IAM/OIDC teardown is separate.

The policy cannot enforce a $20 bill or four-hour teardown. External run deadlines, cost checks and reliable cleanup are still mandatory. `eks:CreateCluster` has no resource-level ARN support; its grant requires the exact run tag, Mumbai region, version, STANDARD support and authentication settings, but AWS provides no cluster-name condition on that action. All subsequent cluster permissions use the exact reserved name. The operator must verify the name before creation; a wrong name could require administrator cleanup. No infrastructure should be launched until the application path and staged IAM/deployment workflow are ready.

## Resource "*" exceptions

Every literal-star grant is inventoried action-by-action in `local-validation.json`. Current AWS service references report no resource types for these granted actions:

- Selected EC2 `Describe*` inventory actions (exact list is in the JSON); `DescribeVpcAttribute` is instead scoped to run-tagged VPCs.
- `eks:CreateCluster`, `eks:ListClusters`, `eks:DescribeAddonVersions`, `eks:DescribeAddonConfiguration`, `eks:DescribeClusterVersions`.
- `autoscaling:DescribeAutoScalingGroups`.
- `elasticloadbalancing:DescribeLoadBalancers`, `elasticloadbalancing:DescribeTags`.
- `cloudformation:ValidateTemplate`, `ecr:GetAuthorizationToken`, `sts:GetCallerIdentity`, `freetier:GetAccountPlanState`.

Regional actions carry `aws:RequestedRegion=ap-south-1`; STS identity and global Free Tier plan reads do not. These read grants can reveal unrelated inventory; AWS cannot scope them by resource ARN. ECR authorization tokens confer only the repository operations otherwise granted. ARN suffix wildcards for not-yet-known resource IDs are separately constrained by account, region, exact parent/run name or mandatory run tags; they are not literal `Resource:"*"` grants.

## Attach in IAM Identity Center

1. Sign in as the authorized Identity Center administrator in the instance's home region. Confirm an **organization instance** with multi-account permissions: account instances cannot provision account permission sets. Do not enable Organizations/change the Free plan automatically if this prerequisite is missing.
2. **Multi-account permissions → Permission sets → Create permission set → Custom permission set**, or open existing `SathonateSprint3Deployer`.
3. Under **Inline policy**, use the JSON editor and paste the complete contents of `SathonateSprint3Deployer.inline.json`. Do not add AdministratorAccess, IAMFullAccess or other broad policies. Save the permission set as `SathonateSprint3Deployer`; use a four-hour session duration if permitted by organization policy. This duration does not stop resources.
4. **AWS accounts → account 429488690613 → Assign users or groups**. Select only the approved Sprint 3 operator/group and this permission set; review and submit. For an existing assignment, open its permission-set/account provisioning status and choose **Update / Reprovision** to apply the changed policy.
5. Sign in through the AWS access portal using this permission set. Locally run `aws configure sso --profile sathonate-sprint3`, select the account and permission set, default region `ap-south-1`, then `aws sso login --profile sathonate-sprint3`. Verify `aws sts get-caller-identity --profile sathonate-sprint3` returns the intended assumed Identity Center role, never root. Enter credentials only in the AWS browser flow.
6. Administrator review: use IAM policy validation/simulation for expected provisioning and cleanup requests and negative tests for foreign tags, other regions, other roles, role editing and unrelated stacks. This is NOT yet live-policy acceptance. Do not broaden permissions in response to errors without identifying the exact resource/action.

## Sources

- [AWS EKS authorization reference](https://docs.aws.amazon.com/service-authorization/latest/reference/list_eks.html).
- [Public AWS service-reference JSON](https://servicereference.us-east-1.amazonaws.com/v1/ec2/ec2.json), used to check EC2 resource/tag authorization and action names across services.
- [CloudFormation caller and service-role behavior](https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/using-iam-servicerole.html).
- [Role passing](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_use_passrole.html).
- [Create an Identity Center permission set](https://docs.aws.amazon.com/singlesignon/latest/userguide/howtocreatepermissionset.html) and [update/provision changes](https://docs.aws.amazon.com/singlesignon/latest/userguide/howtoviewandchangepermissionset.html).
