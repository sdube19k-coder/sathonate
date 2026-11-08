param(
 [string]$ToolsRoot='E:/Sathonate/tools',
 [string]$CacheRoot='E:/Sathonate/.cache'
)
$ErrorActionPreference='Stop'
$repo=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$names=@('PATH','GOOS','GOARCH','SMP_CACHE_ROOT','SMP_KIND_PATH','SMP_QUALIFICATION_PREFIX','SMP_QUALIFICATION_WORKER','SMP_QUALIFICATION_CALICO','SMP_QUALIFICATION_MULTI_NODE','SMP_DYNAMIC_LABEL','SMP_DYNAMIC_EXPAND','SMP_QUALIFICATION_RUN_LABEL')
$prior=@{}
foreach($name in $names){$prior[$name]=[Environment]::GetEnvironmentVariable($name,'Process')}
Push-Location $repo
try {
 $env:PATH=(Join-Path $ToolsRoot 'kubectl-1.34')+';'+$env:PATH
 $env:GOOS=(& go env GOHOSTOS).Trim();$env:GOARCH=(& go env GOHOSTARCH).Trim()
 $env:SMP_CACHE_ROOT=$CacheRoot;$env:SMP_KIND_PATH=Join-Path $ToolsRoot 'kind.exe'
 $env:SMP_QUALIFICATION_PREFIX='sprint6'
 $env:SMP_QUALIFICATION_WORKER='sathonate/fabric-worker:sprint5-recovery'
 $env:SMP_QUALIFICATION_CALICO='1';$env:SMP_QUALIFICATION_MULTI_NODE='1'
 $env:SMP_DYNAMIC_LABEL='a';$env:SMP_DYNAMIC_EXPAND='1'
 $env:SMP_QUALIFICATION_RUN_LABEL='dynamic-sprint6-'+(Get-Date -Format 'yyyyMMdd-HHmmss')+'-'+[guid]::NewGuid().ToString('N').Substring(0,8)
 $evidence=Join-Path '.logs' $env:SMP_QUALIFICATION_RUN_LABEL
 New-Item -ItemType Directory -Path $evidence | Out-Null
 # The Python entry point gates capacity before credential access or creation.
 # The bounded worker pause, stateless rescheduling and original-local-PV
 # recovery suite runs after initial provisioning and customer expansion.
 & python tools/sprint1/qualify.py *> (Join-Path $evidence 'orchestrator.log')
 if($LASTEXITCODE -ne 0){throw "Sprint 6 blocked/failed; inspect $evidence. No capacity bypass is supported."}
 Write-Output "Fixture finished; inspect bounded acceptance and remaining release gates in $evidence."
} finally {
 foreach($name in $names){[Environment]::SetEnvironmentVariable($name,$prior[$name],'Process')}
 Pop-Location
}
