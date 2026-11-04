param([string]$Network='examples/local-kubernetes-network.json',[string]$Namespace='smp-n-acceptance',[string]$Job='smp-acceptance',[string]$Kubeconfig='E:\Sathonate\.cache\smp-qualification.kubeconfig')
$ErrorActionPreference='Stop'
$context=kubectl --kubeconfig $Kubeconfig config current-context
if($context -ne 'kind-smp-qualification'){throw 'This command is restricted to the dedicated local kind context'}
$server=kubectl --kubeconfig $Kubeconfig --context kind-smp-qualification config view --minify -o 'jsonpath={.clusters[0].cluster.server}'
$endpoint=[uri]$server
if($endpoint.Scheme -ne 'https' -or -not $endpoint.IsLoopback){throw 'A loopback API is required'}
$provider=kubectl --kubeconfig $Kubeconfig --context kind-smp-qualification get node smp-qualification-control-plane -o 'jsonpath={.spec.providerID}'
if($provider -ne 'kind://docker/smp-qualification/smp-qualification-control-plane'){throw 'Dedicated Docker kind provider is required'}
if($Namespace -notmatch '^smp-n-[a-z0-9-]+$' -or $Job -notmatch '^smp-[a-z0-9-]+$'){throw 'Invalid local resource name'}
$existing=kubectl --kubeconfig $Kubeconfig --context kind-smp-qualification get namespace $Namespace --ignore-not-found -o json
if($LASTEXITCODE -ne 0){throw 'Cannot inspect namespace ownership'}
if($existing -and ($existing | ConvertFrom-Json).metadata.labels.'fabric.sathonate.io/managed-by' -ne 'fabric-smp-local'){throw 'Refusing an unowned namespace'}
$inputJSON=Get-Content -LiteralPath $Network -Raw
$manifest=@{apiVersion='v1';kind='List';items=@(
 @{apiVersion='v1';kind='Namespace';metadata=@{name=$Namespace;labels=@{'fabric.sathonate.io/managed-by'='fabric-smp-local'}}},
 @{apiVersion='v1';kind='ServiceAccount';metadata=@{name='smp-worker';namespace=$Namespace}},
 @{apiVersion='rbac.authorization.k8s.io/v1';kind='ClusterRoleBinding';metadata=@{name="$Namespace-worker"};roleRef=@{apiGroup='rbac.authorization.k8s.io';kind='ClusterRole';name='cluster-admin'};subjects=@(@{kind='ServiceAccount';name='smp-worker';namespace=$Namespace})},
 @{apiVersion='v1';kind='ConfigMap';metadata=@{name='smp-input';namespace=$Namespace};data=@{'network.json'=$inputJSON}},
 @{apiVersion='batch/v1';kind='Job';metadata=@{name=$Job;namespace=$Namespace};spec=@{backoffLimit=0;activeDeadlineSeconds=1800;template=@{spec=@{serviceAccountName='smp-worker';restartPolicy='Never';securityContext=@{runAsUser=10001;runAsGroup=10001;fsGroup=10001};containers=@(@{name='worker';image='sathonate/fabric-worker:local';imagePullPolicy='Never';workingDir='/workspace';args=@('--namespace',$Namespace);resources=@{requests=@{cpu='100m';memory='256Mi'};limits=@{cpu='2';memory='1Gi'}};volumeMounts=@(@{name='input';mountPath='/inputs';readOnly=$true},@{name='workspace';mountPath='/workspace'})});volumes=@(@{name='input';configMap=@{name='smp-input'}},@{name='workspace';emptyDir=@{medium='Memory';sizeLimit='128Mi'}})}}}}
)}
$manifest | ConvertTo-Json -Depth 30 -Compress | kubectl --kubeconfig $Kubeconfig apply -f -
if($LASTEXITCODE -ne 0){throw 'Could not submit local worker'}
Write-Output "Read progress: kubectl --kubeconfig $Kubeconfig -n $Namespace logs -f job/$Job"
