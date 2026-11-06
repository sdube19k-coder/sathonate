param(
    [ValidateSet('a','b','c','baseline')][string[]]$Topology = @('a','b','c','baseline'),
    [string]$ToolsRoot = 'E:/Sathonate/tools',
    [string]$CacheRoot = 'E:/Sathonate/.cache'
)
$ErrorActionPreference = 'Stop'
$repo = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$buildGoOS = $env:GOOS
$buildGoArch = $env:GOARCH
Set-Location -LiteralPath $repo
New-Item -ItemType Directory -Force .logs | Out-Null
# Scoped to this process/thread; no persistent Windows power setting changes.
Add-Type -TypeDefinition @'
using System.Runtime.InteropServices;
public static class QualificationAwake {
  [DllImport("kernel32.dll")] public static extern uint SetThreadExecutionState(uint flags);
}
'@
if ([QualificationAwake]::SetThreadExecutionState([uint32]2147483649) -eq 0) { throw 'Cannot keep bounded qualification process awake' }
try {
	# Worker builds target Linux; integration test executables must target the
	# host. Do not inherit a previous cross-compilation environment.
	$env:GOOS = (& go env GOHOSTOS).Trim()
	$env:GOARCH = (& go env GOHOSTARCH).Trim()
    $env:PATH = (Join-Path $ToolsRoot 'kubectl-1.34') + ';' + $env:PATH
    $env:SMP_CACHE_ROOT = $CacheRoot
    $env:SMP_KIND_PATH = Join-Path $ToolsRoot 'kind.exe'
    $env:SMP_QUALIFICATION_PREFIX = 'sprint4'
    $env:SMP_QUALIFICATION_WORKER = 'sathonate/fabric-worker:sprint4-dynamic'
    $env:SMP_QUALIFICATION_CALICO = '1'
    foreach ($layout in $Topology) {
        $env:SMP_DYNAMIC_LABEL = if ($layout -eq 'baseline') { '' } else { $layout }
        $env:SMP_DYNAMIC_EXPAND = if ($layout -eq 'a') { '1' } else { '0' }
        $env:SMP_LIFECYCLE_E2E = if ($layout -eq 'baseline') { '1' } else { '0' }
        $attempt = Get-Date -Format 'yyyyMMdd-HHmmss'
        $env:SMP_QUALIFICATION_RUN_LABEL = "dynamic-$layout-$attempt"
        $evidence = Join-Path $repo (".logs/" + $env:SMP_QUALIFICATION_RUN_LABEL)
        New-Item -ItemType Directory -Path $evidence | Out-Null
        Write-Output "Running disposable API qualification: $layout"
        & python tools/sprint1/qualify.py *> (Join-Path $evidence 'orchestrator.log')
        $result = $LASTEXITCODE
        if ($result -ne 0) { throw "Disposable qualification failed: $layout. Evidence: $evidence" }
        Write-Output "Completed qualification and teardown: $layout. Evidence: $evidence"
    }
} finally {
	if ($null -eq $buildGoOS) { Remove-Item Env:GOOS -ErrorAction SilentlyContinue } else { $env:GOOS = $buildGoOS }
	if ($null -eq $buildGoArch) { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue } else { $env:GOARCH = $buildGoArch }
    [void][QualificationAwake]::SetThreadExecutionState([uint32]2147483648)
}
