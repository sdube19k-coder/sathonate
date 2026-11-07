# Minimum remaining local cold/deletion qualification; preserves completed node cases. Never targets the retained cluster.
$ErrorActionPreference = 'Stop'
$backend = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$console = (Resolve-Path (Join-Path $backend '../fabric-console')).Path
$cache = 'E:/Sathonate/.cache'
$kind = 'E:/Sathonate/tools/kind.exe'
$kub = 'E:/Sathonate/tools/kubectl-1.34/kubectl.exe'
$env:PATH = (Split-Path $kub) + ';' + $env:PATH
$engine = & docker info --format '{{.ServerVersion}}' 2>&1
if ($LASTEXITCODE -ne 0) { throw "Docker engine unavailable; no runtime test started: $engine" }
# This single known orphan has an interrupted fixture receipt, not acceptance.
$receipt = Join-Path $console '.logs/dynamic-browser-s5-20261106-225232'
$clusters = @(& $kind get clusters)
if ($LASTEXITCODE -ne 0) { throw 'Local cluster inventory unavailable' }
$configs = @(Get-ChildItem -LiteralPath $cache -Filter 'smp-sprint1-*-runtime.json')
foreach ($runtimeFile in $configs) {
    $runtime = Get-Content -LiteralPath $runtimeFile.FullName -Raw | ConvertFrom-Json
    $name = $runtime.clusterName
    # Only the exact interrupted cluster discovered from this receipt is eligible.
    if ($name -ne 'smp-sprint1-4fd47f55' -or $clusters -notcontains $name) { continue }
    if (!(Test-Path -LiteralPath (Join-Path $receipt 'orchestrator.log'))) { throw 'Interrupted receipt missing' }
    $provider = & $kub --kubeconfig $runtime.kubeconfig --context ('kind-'+$name) --request-timeout=20s get node ($name+'-control-plane') -o 'jsonpath={.spec.providerID}'
    if ($LASTEXITCODE -ne 0 -or $provider -ne ('kind://docker/'+$name+'/'+$name+'-control-plane')) { throw 'Orphan identity not verified; no deletion performed' }
    & $kind delete cluster --name $name
    if ($LASTEXITCODE -ne 0) { throw 'Orphan cleanup failed' }
}
$clusters = @(& $kind get clusters)
if ($LASTEXITCODE -ne 0) { throw 'Local cluster inventory unavailable' }
if (@($clusters | Where-Object {$_ -like 'smp-sprint1-*'}).Count -gt 0) { throw 'Another disposable fixture remains; inspect its ownership and receipt before running another full fixture' }
& docker start smp-postgres | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Dedicated PostgreSQL fixture unavailable' }
$env:SMP_CACHE_ROOT=$cache
$env:GOCACHE=Join-Path $cache 'go-build'
$env:GOPATH=Join-Path $cache 'go'
$env:GOPROXY='off'
$env:GOOS=(& go env GOHOSTOS).Trim()
$env:GOARCH=(& go env GOHOSTARCH).Trim()
$stamp=Get-Date -Format yyyyMMdd-HHmmss
Push-Location $backend
try {
    & go build -o bin/fabric-api-sprint5.exe ./cmd/fabric-api
    if ($LASTEXITCODE -ne 0) { throw 'API build failed' }
    & go build -o bin/fabric-cold-backup.exe ./cmd/fabric-cold-backup
    if ($LASTEXITCODE -ne 0) { throw 'Cold maintenance build failed' }
    if (!(Test-Path -LiteralPath '.logs/sprint5-worker-build.json')) { throw 'Qualified Sprint 5 worker receipt required; preserve the existing pinned image' }
    & python tools/sprint2/postgres.py --output ('.logs/sprint5-resume-postgres-'+$stamp+'.jsonl')
    if ($LASTEXITCODE -ne 0) { throw 'PostgreSQL regressions failed' }
} finally { Pop-Location }
$env:SMP_KIND_PATH=$kind
$env:SMP_BACKEND_ROOT=$backend
$env:SMP_UI_DYNAMIC='1'
$env:SMP_UI_SPRINT5='1'
$env:SMP_UI_SPRINT5_MINIMAL='1'
$env:SMP_UI_SPRINT5_WORKER='1'
$env:SMP_UI_SPRINT5_OPERATIONS='1'
$env:SMP_UI_SPRINT5_OPERATIONS_ONLY='1'
$env:SMP_UI_SPRINT5_COLD_MINIMAL='1'
$env:SMP_UI_CALICO='1'
$env:SMP_UI_EVIDENCE_LABEL='dynamic-browser-s5-'+$stamp
$env:SMP_UI_API_SHA256=(Get-FileHash -LiteralPath (Join-Path $backend 'bin/fabric-api-sprint5.exe') -Algorithm SHA256).Hash.ToLower()
if (-not ('Sprint5ResumeAwake' -as [type])) {
    Add-Type -TypeDefinition 'using System; using System.Runtime.InteropServices; public static class Sprint5ResumeAwake { [DllImport("kernel32.dll")] public static extern uint SetThreadExecutionState(uint flags); }'
}
[Sprint5ResumeAwake]::SetThreadExecutionState(2147483649) | Out-Null
Push-Location $console
try {
    $output=Join-Path '.logs' $env:SMP_UI_EVIDENCE_LABEL
    New-Item -ItemType Directory -Force $output | Out-Null
    & python tools/qualify-ui.py *> (Join-Path $output 'orchestrator.log')
    if ($LASTEXITCODE -ne 0) { throw "Recovery acceptance failed; inspect $output. Automatic cleanup is recorded in ui-qualification.json when the harness completes." }
} finally { Pop-Location; [Sprint5ResumeAwake]::SetThreadExecutionState(2147483648) | Out-Null }
