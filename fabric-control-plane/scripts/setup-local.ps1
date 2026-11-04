param([string]$Workspace='E:\Sathonate')
$ErrorActionPreference='Stop'
$PSNativeCommandUseErrorActionPreference=$true
$root=Split-Path -Parent $PSScriptRoot
$cache=Join-Path $Workspace '.cache'
$privateDirectory=Join-Path $cache 'local-runtime'
$tools=Join-Path $Workspace 'tools'
$dependencies=Join-Path $Workspace 'dependencies'
New-Item -ItemType Directory -Force -Path $privateDirectory,$tools,$dependencies | Out-Null
foreach($command in @('docker','git','go','kubectl')){if(-not(Get-Command $command -ErrorAction SilentlyContinue)){throw "Install prerequisite: $command"}}
docker info --format '{{.OSType}}' | Out-Null
if($LASTEXITCODE -ne 0){throw 'Start Docker Desktop with its Linux engine'}
$kind=Join-Path $tools 'kind.exe'
if(-not(Test-Path -LiteralPath $kind)){
 $download=Join-Path $cache 'kind-windows-amd64'
 Invoke-WebRequest https://github.com/kubernetes-sigs/kind/releases/download/v0.33.0/kind-windows-amd64 -OutFile $download
 $checksumFile=Join-Path $cache 'kind-windows-amd64.sha256sum'
 Invoke-WebRequest https://github.com/kubernetes-sigs/kind/releases/download/v0.33.0/kind-windows-amd64.sha256sum -OutFile $checksumFile
 $checksum=((Get-Content -LiteralPath $checksumFile -Raw).Trim() -split '\s+')[0]
 if($checksum -notmatch '^[a-fA-F0-9]{64}$'){throw 'Invalid kind release checksum'}
 if((Get-FileHash -LiteralPath $download -Algorithm SHA256).Hash.ToLowerInvariant() -ne $checksum.ToLowerInvariant()){throw 'kind checksum mismatch'}
 Copy-Item -LiteralPath $download -Destination $kind
}
$operator=Join-Path $dependencies 'bevel-operator-fabric'
if(-not(Test-Path -LiteralPath $operator)){git clone --branch v1.14.0 --depth 1 https://github.com/hyperledger-bevel/bevel-operator-fabric.git $operator}
if((git -C $operator rev-parse HEAD) -ne 'cd236ccc61d1d159b3dadb4625d08bb5682a8451'){throw 'Unexpected Bevel operator revision'}
$kubeconfig=Join-Path $cache 'smp-qualification.kubeconfig'
$clusters=& $kind get clusters
if($clusters -notcontains 'smp-qualification'){& $kind create cluster --name smp-qualification --config (Join-Path $root 'tools\kind.yaml') --kubeconfig $kubeconfig --wait 180s}
else {docker start smp-qualification-control-plane | Out-Null; & $kind get kubeconfig --name smp-qualification | Set-Content -LiteralPath $kubeconfig -Encoding utf8}
$identity=[Security.Principal.WindowsIdentity]::GetCurrent().Name
icacls $kubeconfig /inheritance:r /grant:r "${identity}:F" | Out-Null
kubectl --kubeconfig $kubeconfig --context kind-smp-qualification wait --for=condition=Ready node/smp-qualification-control-plane --timeout=180s
Push-Location -LiteralPath $root
try {
 docker build -t sathonate/bevel-tools:dev-20261016 tools/bevel
 & "$PSScriptRoot\bootstrap-operator.ps1" -Workspace $Workspace
 $env:GOCACHE=Join-Path $cache 'go-build'; $env:GOPATH=Join-Path $cache 'go'
 $previousGOOS=$env:GOOS; $previousGOARCH=$env:GOARCH; $previousCGO=$env:CGO_ENABLED
 try {
  $env:GOOS='linux';$env:GOARCH='amd64';$env:CGO_ENABLED='0'
  go build -o bin/fabric-worker ./cmd/fabric-worker
  go build -o bin/local-chaincode ./cmd/local-chaincode
 } finally {$env:GOOS=$previousGOOS;$env:GOARCH=$previousGOARCH;$env:CGO_ENABLED=$previousCGO}
 python tools/kubernetes/build-scoped-cli.py --source $operator --build-dir (Join-Path $cache "bevel-scoped-cli") --output bin/kubectl-hlf-scoped
 if($LASTEXITCODE -ne 0){throw 'Protected Bevel CLI build failed'}
 docker build -f tools/kubernetes/Dockerfile --target worker -t sathonate/fabric-worker:local .
 docker build -f tools/kubernetes/Dockerfile --target smoke -t sathonate/smp-smoke:local .
 & $kind load docker-image sathonate/fabric-worker:local sathonate/smp-smoke:local --name smp-qualification
 $passwordFile=Join-Path $privateDirectory 'postgres-password'
 if(-not(Test-Path -LiteralPath $passwordFile)){
  $random=New-Object byte[] 32
  $rng=[Security.Cryptography.RandomNumberGenerator]::Create()
  try {$rng.GetBytes($random)} finally {$rng.Dispose()}
  [IO.File]::WriteAllText($passwordFile,([BitConverter]::ToString($random).Replace('-','').ToLowerInvariant()))
  icacls $passwordFile /inheritance:r /grant:r "${identity}:F" | Out-Null
 }
 $existing=docker ps -a --filter name=^/smp-postgres$ --format '{{.Names}}'
 if($existing -eq 'smp-postgres'){
  $inspection=(docker inspect smp-postgres | ConvertFrom-Json)[0]
  if($inspection.Config.Image -ne 'postgres:17.6' -or -not($inspection.Mounts | Where-Object {$_.Name -eq 'smp-postgres-data'})){throw 'Reserved PostgreSQL container has unexpected configuration'}
  docker start smp-postgres | Out-Null
 } else {
  docker volume create smp-postgres-data | Out-Null
  docker run -d --name smp-postgres -p 127.0.0.1:15432:5432 -e POSTGRES_USER=smp -e POSTGRES_DB=fabric_smp -e POSTGRES_PASSWORD_FILE=/run/secrets/postgres-password --mount "type=bind,source=$passwordFile,target=/run/secrets/postgres-password,readonly" --mount type=volume,source=smp-postgres-data,target=/var/lib/postgresql/data postgres:17.6 | Out-Null
 }
 $ready=$false
 $nativeErrors=$PSNativeCommandUseErrorActionPreference
 try {
  $PSNativeCommandUseErrorActionPreference=$false
  for($attempt=0;$attempt -lt 30;$attempt++){
   docker exec smp-postgres pg_isready -U smp -d fabric_smp *> $null
   if($LASTEXITCODE -eq 0){$ready=$true;break}; Start-Sleep -Seconds 1
  }
 } finally {$PSNativeCommandUseErrorActionPreference=$nativeErrors}
 if(-not $ready){throw 'PostgreSQL did not become ready'}
 & "$PSScriptRoot\run-local.ps1" -Kubeconfig $kubeconfig -PasswordFile $passwordFile -Migrate
 Write-Output 'Local infrastructure is ready. Run scripts/run-local.ps1, then start the separate React repository.'
} finally {Pop-Location}
