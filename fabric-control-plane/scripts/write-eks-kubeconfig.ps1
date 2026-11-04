param(
 [Parameter(Mandatory)][string]$AwsProfile,
 [Parameter(Mandatory)][string]$Region,
 [Parameter(Mandatory)][string]$Cluster,
 [Parameter(Mandatory)][string]$Context,
 [Parameter(Mandatory)][string]$AccountID,
 [Parameter(Mandatory)][string]$RoleARN,
 [Parameter(Mandatory)][string]$Output
)
$ErrorActionPreference='Stop'
foreach($value in @($AwsProfile,$Cluster,$Context)){if($value -notmatch '^[a-z][a-z0-9-]{0,62}$'){throw 'Invalid explicit profile/cluster/context identifier'}}
if($Region -notmatch '^[a-z]{2}-[a-z]+-[0-9]+$' -or $AccountID -notmatch '^[0-9]{12}$' -or $RoleARN -notmatch "^arn:aws:iam::${AccountID}:role/[A-Za-z0-9+=,.@_/-]+$"){throw 'Invalid approved account/region/role'}
if(Test-Path -LiteralPath $Output){throw 'Refusing to overwrite an existing kubeconfig; choose a new private path'}
$identityJSON=& aws --profile $AwsProfile --region $Region --no-cli-pager sts get-caller-identity 2>$null
if($LASTEXITCODE -ne 0){throw 'Cannot verify assumed-role identity; sign in locally'}
$identity=$identityJSON | ConvertFrom-Json
$roleName=($RoleARN -split '/')[-1]
if($identity.Account -ne $AccountID -or -not $identity.Arn.StartsWith("arn:aws:sts::${AccountID}:assumed-role/${roleName}/")){throw 'AWS identity is not the approved assumed role'}
$clusterJSON=& aws --profile $AwsProfile --region $Region --no-cli-pager eks describe-cluster --name $Cluster 2>$null
if($LASTEXITCODE -ne 0){throw 'Cannot inspect approved existing EKS cluster'}
$target=($clusterJSON | ConvertFrom-Json).cluster
if($target.arn -ne "arn:aws:eks:${Region}:${AccountID}:cluster/${Cluster}" -or $target.status -ne 'ACTIVE' -or -not $target.certificateAuthority.data){throw 'Existing active EKS binding could not be verified'}
$config=@{
 apiVersion='v1';kind='Config';'current-context'=$Context
 clusters=@(@{name=$Context;cluster=@{server=$target.endpoint;'certificate-authority-data'=$target.certificateAuthority.data}})
 contexts=@(@{name=$Context;context=@{cluster=$Context;user=$Context}})
 users=@(@{name=$Context;user=@{exec=@{apiVersion='client.authentication.k8s.io/v1beta1';command='aws';args=@('--profile',$AwsProfile,'--region',$Region,'eks','get-token','--cluster-name',$Cluster);interactiveMode='Never';provideClusterInfo=$false}}})
}
$config | ConvertTo-Json -Depth 15 | Set-Content -LiteralPath $Output -Encoding utf8
$owner=[Security.Principal.WindowsIdentity]::GetCurrent().Name
icacls $Output /inheritance:r /grant:r "${owner}:F" | Out-Null
if($LASTEXITCODE -ne 0){throw 'Could not restrict kubeconfig ACL; restrict access before using it'}
Write-Output 'Created explicit role-bound kubeconfig. No AWS resource was created; no credentials were written.'
