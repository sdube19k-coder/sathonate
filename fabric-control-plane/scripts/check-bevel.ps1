param(
    [Parameter(Mandatory=$true)][string]$BevelCheckout,
    [switch]$Build
)
$ErrorActionPreference = 'Stop'
$expectedCommit = '8824ddc9c461f8d427ab2b140fb56211d89d4ebe'
$checkout = (Resolve-Path -LiteralPath $BevelCheckout).Path
$commit = git -C $checkout rev-parse HEAD
if ($LASTEXITCODE -ne 0 -or $commit -ne $expectedCommit) { throw 'Bevel source does not match the inspected commit' }
$image = 'sathonate/bevel-tools:dev-20261016'
if ($Build) {
    docker build --tag $image (Join-Path $PSScriptRoot '../tools/bevel')
    if ($LASTEXITCODE -ne 0) { throw 'Toolchain image build failed' }
}
docker run --rm --read-only --tmpfs /tmp:rw,nosuid,size=256m --env ANSIBLE_HOME=/tmp/ansible --cap-drop ALL --security-opt no-new-privileges $image sh -ec 'ansible-playbook --version; helm version --short; kubectl version --client=true; ansible-galaxy collection list'
if ($LASTEXITCODE -ne 0) { throw 'Toolchain verification failed' }
docker run --rm --read-only --tmpfs /tmp:rw,nosuid,size=256m --env ANSIBLE_HOME=/tmp/ansible --cap-drop ALL --security-opt no-new-privileges --mount "type=bind,source=$checkout,target=/bevel,readonly" --workdir /bevel/platforms/hyperledger-fabric/configuration $image ansible-playbook deploy-network.yaml --syntax-check -e disableCallBack=true
if ($LASTEXITCODE -ne 0) { throw 'Bevel playbook syntax validation failed' }
Write-Output 'Toolchain and playbook syntax verified. This does not deploy a network or qualify cloud execution.'
if (Select-String -LiteralPath (Join-Path $checkout 'platforms/hyperledger-fabric/configuration/roles/helm_component/templates/orderernode.tpl') -SimpleMatch 'helm.fluxcd.io/v1' -Quiet) {
    Write-Warning 'Cloud execution blocked: inspected fork still generates Flux v1 HelmRelease resources.'
}
