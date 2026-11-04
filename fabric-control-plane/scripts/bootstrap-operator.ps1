param([string]$Workspace='E:\Sathonate')
$ErrorActionPreference='Stop'
$source=Join-Path $Workspace 'dependencies\bevel-operator-fabric'
$revision=git -C $source rev-parse HEAD
if($revision -ne 'cd236ccc61d1d159b3dadb4625d08bb5682a8451'){throw 'Bevel operator checkout must match the qualified commit'}
$privateDirectory=Join-Path $Workspace '.cache\local-runtime'
New-Item -ItemType Directory -Force -Path $privateDirectory | Out-Null
$internalConfig=Join-Path $privateDirectory 'kind-internal.kubeconfig'
try {
 & (Join-Path $Workspace 'tools\kind.exe') get kubeconfig --name smp-qualification --internal | Set-Content -LiteralPath $internalConfig -Encoding utf8
 if($LASTEXITCODE -ne 0){throw 'Cannot read dedicated kind configuration'}
 $identity=[Security.Principal.WindowsIdentity]::GetCurrent().Name
 icacls $internalConfig /inheritance:r /grant:r "${identity}:F" | Out-Null
 $playbook=Join-Path (Split-Path -Parent $PSScriptRoot) 'tools\kubernetes\bootstrap.yaml'
 docker run --rm --network kind --mount "type=bind,source=$source,target=/operator,readonly" --mount "type=bind,source=$playbook,target=/bootstrap.yaml,readonly" --mount "type=bind,source=$internalConfig,target=/kubeconfig,readonly" -e KUBECONFIG=/kubeconfig -e HELM_CACHE_HOME=/tmp/helm/cache -e HELM_CONFIG_HOME=/tmp/helm/config -e HELM_DATA_HOME=/tmp/helm/data sathonate/bevel-tools:dev-20261016 ansible-playbook /bootstrap.yaml
 if($LASTEXITCODE -ne 0){throw 'Operator bootstrap failed'}
} finally { if(Test-Path -LiteralPath $internalConfig){Remove-Item -LiteralPath $internalConfig} }
