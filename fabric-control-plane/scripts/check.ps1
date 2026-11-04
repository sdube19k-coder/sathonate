$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$cacheRoot = Join-Path (Split-Path -Parent $projectRoot) '.cache'
$env:GOCACHE = Join-Path $cacheRoot 'go-build'
$env:GOPATH = Join-Path $cacheRoot 'go'
Push-Location -LiteralPath $projectRoot
try {
    $formatIssues = @(gofmt -l cmd internal)
    if ($LASTEXITCODE -ne 0 -or $formatIssues.Count -gt 0) { throw "Run gofmt: $formatIssues" }
    go test ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed' }
    go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go vet failed' }
    & (Join-Path $PSScriptRoot 'build-contract.ps1')
    New-Item -ItemType Directory -Path bin -Force | Out-Null
    go build -o bin/fabric-api.exe ./cmd/fabric-api
    if ($LASTEXITCODE -ne 0) { throw 'API build failed' }
    go build -o bin/fabricctl.exe ./cmd/fabricctl
    if ($LASTEXITCODE -ne 0) { throw 'CLI build failed' }
} finally {
    Pop-Location
}
