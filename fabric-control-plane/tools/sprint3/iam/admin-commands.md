# Administrator command reference

Start with the [current manual setup and AWS runbook](../../../docs/manual-setup-and-aws-runbook.md).
The role and empty-repository bootstrap completed previously. Live issuer
binding and cloud teardown remain open. The commands below use an example
administrator profile. For the current run use `sathonate-bootstrap`.

Use PowerShell 7 and a separately approved **non-root administrator SSO profile**. The ordinary deployer deliberately cannot create IAM roles or ECR repositories. Replace `approved-admin-sso` with that administrator profile.

```powershell
Set-Location E:\Sathonate\fabric-control-plane
# Offline plan, no AWS calls:
./tools/sprint3/iam/admin-bootstrap.ps1 -AdminProfile approved-admin-sso -Action Plan
# Administrator-only bootstrap after review:
./tools/sprint3/iam/admin-bootstrap.ps1 -AdminProfile approved-admin-sso -Action Bootstrap
```

Bootstrap creates only the four exact run-tagged roles, the immutable AES256 private repository `sathonate-s3-20261027`, and missing standard service-linked roles. Existing roles/repository must have the correct tag and expected trust/policies/security settings; unexpected objects are not adopted. Add `-PermissionsBoundaryArn <existing-approved-boundary-ARN>` if organization policy requires it. Managed policy ARNs and exact service trust JSON files are listed in `sprint3-admin-bootstrap.md`.

CNI/EBS roles initially receive a deny-only trust, so they cannot be assumed. No cluster is needed to perform this bootstrap. **Do not create the cluster while billing remains unresolved.** Once cluster-only deployment is separately allowed, the administrator binds both roles to its exact issuer:

```powershell
$issuer = aws eks describe-cluster --name sathonate-s3-20261027 --profile approved-admin-sso --region ap-south-1 --query cluster.identity.oidc.issuer --output text
./tools/sprint3/iam/admin-bootstrap.ps1 -AdminProfile approved-admin-sso -Action BindIssuer -Issuer $issuer
```

The script checks the issuer against the signed live cluster response, creates only its run-tagged OIDC provider with audience `sts.amazonaws.com`, and sets exact subjects `system:serviceaccount:kube-system:aws-node` and `system:serviceaccount:kube-system:ebs-csi-controller-sa`. It refuses unexpected existing trusts. Record the issuer for cleanup. No wildcard federation trust is granted. Coordinate the administrator phase in advance; do not leave a chargeable cluster waiting for input.

After verified EKS/EC2/EBS/VPC teardown:

```powershell
./tools/sprint3/iam/admin-bootstrap.ps1 -AdminProfile approved-admin-sso -Action Cleanup -Issuer $issuer
```

Cleanup refuses while the cluster exists, verifies ownership tags, rejects unexpected role policies or instance profiles, and preserves shared service-linked roles. Only the exact tagged run repository can be force-deleted. If no cluster/OIDC was ever created, omit `-Issuer` to remove bootstrap-only roles/ECR; OIDC inventory is then explicitly NOT VERIFIED. This helper does not perform billable-resource teardown and does not claim complete cloud cleanup.

## Read-only billing diagnosis

These require administrator read permissions; do not broaden the ordinary deployer automatically:

```powershell
aws billing get-credits --account-id 429488690613 --start-date 2026-10-01T00:00:00Z --profile approved-admin-sso --region us-east-1 --query 'credits[].{Type:creditType,Status:creditStatus,Initial:initialAmount,Remaining:remainingAmount,Expiry:endDate,Products:applicableProductNames}' --output json
aws organizations describe-organization --profile approved-admin-sso --region us-east-1 --output json
```

These need `billing:GetCredits` and `organizations:DescribeOrganization`, respectively. Both reads were denied to the deployer; no permissions were changed. In **Billing and Cost Management → Credits**, verify active balance, expiry and applicability to EKS, EC2, EBS and associated services. Review account audit history for the plan change and Organizations creation/join.

The latest Free Tier API reports **PAID / ACTIVE / $0 plan credits**. Promotional credits remain **UNKNOWN**, not assumed zero and not assumed available. AWS documents that joining Organizations or setting up Control Tower immediately expires Free Tier credits. That is a possible explanation for this account's change, not a confirmed cause. See [Free Tier FAQ](https://docs.aws.amazon.com/awsaccountbilling/latest/aboutv2/free-tier-FAQ.html) and [GetCredits](https://docs.aws.amazon.com/aws-cost-management/latest/APIReference/API_billing_GetCredits.html).
