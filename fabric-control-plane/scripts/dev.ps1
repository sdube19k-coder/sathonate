$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$cacheRoot = Join-Path (Split-Path -Parent $projectRoot) '.cache'
$env:GOCACHE = Join-Path $cacheRoot 'go-build'
$env:GOPATH = Join-Path $cacheRoot 'go'
Push-Location -LiteralPath $projectRoot
try {
    & (Join-Path $PSScriptRoot 'build-contract.ps1')
    go run ./cmd/fabric-api
    if ($LASTEXITCODE -ne 0) { throw 'Control plane stopped with an error' }
} finally {
    Pop-Location
}
