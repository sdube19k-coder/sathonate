param(
 [Parameter(Mandatory)][ValidatePattern('^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$')][string]$AdminProfile,
 [ValidateSet('Plan','Bootstrap','BindIssuer','Cleanup')][string]$Action='Plan',
 [string]$Issuer='',
 [ValidatePattern('^$|^arn:aws:iam::429488690613:policy/[A-Za-z0-9+=,.@_/-]+$')][string]$PermissionsBoundaryArn=''
)
$ErrorActionPreference='Stop'
$account='429488690613'; $region='ap-south-1'; $run='sathonate-s3-20261027'
$aws=Join-Path $env:LOCALAPPDATA 'Programs/Amazon/AWSCLIV2/aws.exe'
$roles=@{
 clusterrole=@('arn:aws:iam::aws:policy/AmazonEKSClusterPolicy')
 noderole=@('arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy','arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryPullOnly')
 cnirole=@('arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy')
 ebsrole=@('arn:aws:iam::aws:policy/service-role/AmazonEBSCSIDriverPolicy')
}
if($Action -eq 'Plan'){
 Write-Output 'PLAN ONLY: no AWS calls. Bootstrap creates only four tagged roles, missing standard service-linked roles and one immutable AES256 ECR repository.'
 Write-Output 'BindIssuer requires the live exact cluster issuer; Cleanup refuses while the cluster exists. Administrator assumed-role profile only.'
 $roles.GetEnumerator() | ForEach-Object {Write-Output ("$run-$($_.Key): "+($_.Value -join ', '))}
 exit 0
}
function Invoke-Aws([string[]]$Arguments,[string]$Missing='',[string]$Profile=$AdminProfile){
 # Windows PowerShell reports redirected native stderr as PowerShell errors.
 # Let the CLI return so expected missing resources can be handled by exit code.
 $ErrorActionPreference='Continue'
 $PSNativeCommandUseErrorActionPreference=$false
 $raw=& $aws @Arguments --profile $Profile --region $region --output json --no-cli-pager 2>&1
 if($LASTEXITCODE -ne 0){
  if($Missing -and (($raw -join "`n") -match $Missing)){return $null}
  throw ('AWS administrator operation failed: '+($Arguments[0..1] -join ' ')+". `n"+($raw -join "`n"))
 }
 if($raw){return (($raw -join "`n")|ConvertFrom-Json)}
}
function Canonical($Value){
 if($null -eq $Value){return 'null'}
 if($Value -is [System.Collections.IDictionary]){
  $ordered=[ordered]@{};foreach($key in ($Value.Keys|Sort-Object)){$ordered[$key]=Canonical $Value[$key]};return ($ordered|ConvertTo-Json -Compress -Depth 30)
 }
 if($Value -is [pscustomobject]){
  $properties=@{}
  foreach($property in $Value.PSObject.Properties){$properties[$property.Name]=$property.Value}
  return Canonical $properties
 }
 if($Value -is [array]){return '['+(($Value|ForEach-Object {Canonical $_}) -join ',')+']'}
 return ($Value|ConvertTo-Json -Compress -Depth 30)
}
function Assert-RunTag($Tags){
 if(@($Tags|Where-Object {$_.Key -eq 'SathonateRun' -and $_.Value -eq $run}).Count -ne 1){throw 'Run ownership tag missing; refusing adoption or modification'}
}
function Write-PublicJson($Value){
 $path=Join-Path ([IO.Path]::GetTempPath()) ('sathonate-admin-'+[guid]::NewGuid().ToString('N')+'.json')
 [IO.File]::WriteAllText($path,($Value|ConvertTo-Json -Depth 30),[Text.UTF8Encoding]::new($false));return $path
}
function Assert-RolePolicies([string]$Name,[string[]]$Expected){
 $attached=Invoke-Aws @('iam','list-attached-role-policies','--role-name',$Name)
 foreach($policy in $attached.AttachedPolicies){if($policy.PolicyArn -notin $Expected){throw 'Unexpected attached role policy; administrator review required'}}
 $inline=Invoke-Aws @('iam','list-role-policies','--role-name',$Name)
 if(@($inline.PolicyNames).Count -ne 0){throw 'Unexpected inline role policy; administrator review required'}
 return $attached
}
$who=Invoke-Aws @('sts','get-caller-identity')
if($who.Account -ne $account -or $who.Arn -notmatch '^arn:aws:sts::429488690613:assumed-role/'){throw 'Approved account assumed-role administrator identity required; root is forbidden'}
# A deny-only policy with a concrete account principal grants no trust.
$deny=@{Version='2012-10-17';Statement=@(@{Effect='Deny';Principal=@{AWS="arn:aws:iam::${account}:root"};Action='sts:AssumeRole'})}
if($Action -eq 'Bootstrap'){
 foreach($kind in $roles.Keys){
  $name="$run-$kind"
  if($kind -eq 'clusterrole'){$trust=Get-Content -Raw (Join-Path $PSScriptRoot 'sprint3-cluster-trust.json')|ConvertFrom-Json}
  elseif($kind -eq 'noderole'){$trust=Get-Content -Raw (Join-Path $PSScriptRoot 'sprint3-node-trust.json')|ConvertFrom-Json}
  else{$trust=$deny} # addon placeholders cannot be assumed before verified OIDC binding
  $existing=Invoke-Aws @('iam','get-role','--role-name',$name) 'NoSuchEntity'
  if($existing){
   Assert-RunTag $existing.Role.Tags
   if($PermissionsBoundaryArn -and $existing.Role.PermissionsBoundary.PermissionsBoundaryArn -ne $PermissionsBoundaryArn){throw 'Existing permissions boundary differs'}
   if((Canonical $existing.Role.AssumeRolePolicyDocument) -ne (Canonical $trust)){throw 'Existing role trust differs; refusing overwrite'}
  }else{
   $file=Write-PublicJson $trust
   $create=@('iam','create-role','--role-name',$name,'--assume-role-policy-document',"file://$file",'--tags',"Key=SathonateRun,Value=$run")
   if($PermissionsBoundaryArn){$create+=@('--permissions-boundary',$PermissionsBoundaryArn)}
   try{$null=Invoke-Aws $create}
   finally{Remove-Item -LiteralPath $file}
  }
  $attached=Assert-RolePolicies $name $roles[$kind]
  foreach($policy in $roles[$kind]){if($policy -notin $attached.AttachedPolicies.PolicyArn){$null=Invoke-Aws @('iam','attach-role-policy','--role-name',$name,'--policy-arn',$policy)}}
 }
 foreach($service in @(@('AWSServiceRoleForAmazonEKS','eks.amazonaws.com'),@('AWSServiceRoleForAmazonEKSNodegroup','eks-nodegroup.amazonaws.com'),@('AWSServiceRoleForAutoScaling','autoscaling.amazonaws.com'))){
  if(-not(Invoke-Aws @('iam','get-role','--role-name',$service[0]) 'NoSuchEntity')){$null=Invoke-Aws @('iam','create-service-linked-role','--aws-service-name',$service[1])}
 }
 $repo=Invoke-Aws @('ecr','describe-repositories','--repository-names',$run) 'RepositoryNotFoundException'
 if($repo){
  $tags=Invoke-Aws @('ecr','list-tags-for-resource','--resource-arn',"arn:aws:ecr:${region}:${account}:repository/$run");Assert-RunTag $tags.tags
  if($repo.repositories[0].imageTagMutability -ne 'IMMUTABLE' -or $repo.repositories[0].encryptionConfiguration.encryptionType -ne 'AES256'){throw 'Existing repository security configuration differs'}
 }else{$null=Invoke-Aws @('ecr','create-repository','--repository-name',$run,'--image-tag-mutability','IMMUTABLE','--encryption-configuration','encryptionType=AES256','--tags',"Key=SathonateRun,Value=$run")}
 Write-Output 'Administrator bootstrap complete. No EKS/EC2 resource created. Addon roles remain non-assumable until exact issuer binding.'
 exit 0
}
if($Issuer){
 if($Issuer -notmatch '^https://oidc\.eks\.ap-south-1\.amazonaws\.com/id/[A-Fa-f0-9]+$'){throw 'Verified exact cluster OIDC issuer URL required'}
 $provider=$Issuer.Substring(8);$providerArn="arn:aws:iam::${account}:oidc-provider/$provider"
}elseif($Action -eq 'BindIssuer'){throw 'Verified exact cluster OIDC issuer URL required'}
if($Action -eq 'BindIssuer'){
 $cluster=Invoke-Aws @('eks','describe-cluster','--name',$run)
 if($cluster.cluster.arn -ne "arn:aws:eks:${region}:${account}:cluster/$run" -or $cluster.cluster.identity.oidc.issuer -ne $Issuer){throw 'Issuer not bound to approved live cluster'}
 $oidc=Invoke-Aws @('iam','get-open-id-connect-provider','--open-id-connect-provider-arn',$providerArn) 'NoSuchEntity'
 if($oidc){Assert-RunTag $oidc.Tags;if(@($oidc.ClientIDList).Count -ne 1 -or $oidc.ClientIDList[0] -ne 'sts.amazonaws.com'){throw 'OIDC audience differs'}}
 else{$null=Invoke-Aws @('iam','create-open-id-connect-provider','--url',$Issuer,'--client-id-list','sts.amazonaws.com','--tags',"Key=SathonateRun,Value=$run")}
 foreach($kind in @('cnirole','ebsrole')){
  $name="$run-$kind";$sa=if($kind -eq 'cnirole'){'aws-node'}else{'ebs-csi-controller-sa'}
  $conditions=@{};$conditions["${provider}:aud"]='sts.amazonaws.com';$conditions["${provider}:sub"]="system:serviceaccount:kube-system:$sa"
  $trust=@{Version='2012-10-17';Statement=@(@{Effect='Allow';Principal=@{Federated=$providerArn};Action='sts:AssumeRoleWithWebIdentity';Condition=@{StringEquals=$conditions}})}
  $existing=Invoke-Aws @('iam','get-role','--role-name',$name);Assert-RunTag $existing.Role.Tags;$null=Assert-RolePolicies $name $roles[$kind]
  $actual=Canonical $existing.Role.AssumeRolePolicyDocument
  if($actual -ne (Canonical $deny) -and $actual -ne (Canonical $trust)){throw 'Unexpected addon trust; refusing rebinding'}
  $file=Write-PublicJson $trust
  try{$null=Invoke-Aws @('iam','update-assume-role-policy','--role-name',$name,'--policy-document',"file://$file")}
  finally{Remove-Item -LiteralPath $file}
 }
 Write-Output 'Only the two run-owned addon trusts were bound to the verified exact issuer and service accounts.'
 exit 0
}
# Cleanup is administrator-invoked only, after separate billable-resource teardown.
$cluster=Invoke-Aws @('eks','describe-cluster','--name',$run) 'ResourceNotFoundException'
if($cluster){throw 'Cluster still exists: stop and complete billable-resource teardown before IAM/OIDC cleanup'}
$repo=Invoke-Aws @('ecr','describe-repositories','--repository-names',$run) 'RepositoryNotFoundException'
if($repo){
 $tags=Invoke-Aws @('ecr','list-tags-for-resource','--resource-arn',"arn:aws:ecr:${region}:${account}:repository/$run")
 Assert-RunTag $tags.tags
 $deployer=Invoke-Aws @('sts','get-caller-identity') '' 'sathonate-sprint3'
 if($deployer.Account -ne $account -or $deployer.Arn -notmatch '^arn:aws:sts::429488690613:assumed-role/AWSReservedSSO_Sprint3-permissions_[a-fA-F0-9]{16}/sathonate-sprint3$'){throw 'Approved deployer identity required for ECR cleanup'}
 $null=Invoke-Aws @('ecr','delete-repository','--repository-name',$run,'--force') 'RepositoryNotFoundException' 'sathonate-sprint3'
 if(Invoke-Aws @('ecr','describe-repositories','--repository-names',$run) 'RepositoryNotFoundException'){throw 'ECR cleanup not verified'}
}
foreach($kind in $roles.Keys){
 $name="$run-$kind";$role=Invoke-Aws @('iam','get-role','--role-name',$name) 'NoSuchEntity'
 if(-not $role){continue};Assert-RunTag $role.Role.Tags;$attached=Assert-RolePolicies $name $roles[$kind]
 $profiles=Invoke-Aws @('iam','list-instance-profiles-for-role','--role-name',$name)
 if(@($profiles.InstanceProfiles).Count){throw 'Instance profile still references role; verify nodegroup teardown first'}
 foreach($policy in $attached.AttachedPolicies){$null=Invoke-Aws @('iam','detach-role-policy','--role-name',$name,'--policy-arn',$policy.PolicyArn)}
 $null=Invoke-Aws @('iam','delete-role','--role-name',$name)
}
if($providerArn){
 $oidc=Invoke-Aws @('iam','get-open-id-connect-provider','--open-id-connect-provider-arn',$providerArn) 'NoSuchEntity'
 if($oidc){Assert-RunTag $oidc.Tags;$null=Invoke-Aws @('iam','delete-open-id-connect-provider','--open-id-connect-provider-arn',$providerArn)}
}else{Write-Output 'No issuer supplied: OIDC inventory/cleanup NOT VERIFIED. Supply the recorded exact issuer if BindIssuer was ever executed.'}
Write-Output 'Run-owned IAM/OIDC/ECR cleanup commands completed. Shared service-linked roles preserved. Independently verify billable-resource inventory.'
