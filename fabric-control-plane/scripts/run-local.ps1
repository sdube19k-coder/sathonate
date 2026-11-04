param([string]$Kubeconfig='E:\Sathonate\.cache\smp-qualification.kubeconfig',[string]$PasswordFile='E:\Sathonate\.cache\local-runtime\postgres-password',[switch]$Migrate)
$ErrorActionPreference='Stop'
$projectRoot=Split-Path -Parent $PSScriptRoot
$env:GOCACHE=Join-Path (Split-Path -Parent $projectRoot) '.cache\go-build'
$env:GOPATH=Join-Path (Split-Path -Parent $projectRoot) '.cache\go'
$password=[IO.File]::ReadAllText((Resolve-Path -LiteralPath $PasswordFile).Path).Trim()
$env:DATABASE_URL="postgres://smp:$password@127.0.0.1:15432/fabric_smp?sslmode=disable"
Push-Location -LiteralPath $projectRoot
try {
 if($Migrate){go run ./cmd/fabric-api -migrate}else{go run ./cmd/fabric-api -deployment-mode local-kubernetes -local-kubeconfig $Kubeconfig}
 if($LASTEXITCODE -ne 0){throw 'Local control plane exited with an error'}
} finally {Remove-Item Env:DATABASE_URL -ErrorAction SilentlyContinue;Pop-Location}
