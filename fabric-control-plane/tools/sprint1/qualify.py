"""Disposable orchestration/inspection only. Deployment originates in the Go HTTP API test.
No CA issuance, Fabric deployment or transaction is implemented by this script.
"""
import base64,gzip,hashlib,json,os,re,ssl,subprocess,sys,tarfile,time,uuid
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
CACHE=Path(os.environ.get('SMP_CACHE_ROOT',str(ROOT.parent/'.cache'))).resolve()
KIND=Path(os.environ.get('SMP_KIND_PATH',str(ROOT.parent/'tools/kind.exe'))).resolve()
NAME='smp-sprint1-'+uuid.uuid4().hex[:8]
CONFIG=CACHE/(NAME+'.kubeconfig')
RUNTIME=CACHE/(NAME+'-runtime.json')
PREFIX=os.environ.get('SMP_QUALIFICATION_PREFIX','sprint1')
if PREFIX not in ('sprint1','sprint2','sprint3','sprint4','sprint6'):raise RuntimeError('Invalid qualification prefix')
WORKER=os.environ.get('SMP_QUALIFICATION_WORKER','sathonate/fabric-worker:secure-local')
if WORKER not in ('sathonate/fabric-worker:secure-local','sathonate/fabric-worker:sprint2','sathonate/fabric-worker:sprint3','sathonate/fabric-worker:sprint4-lifecycle','sathonate/fabric-worker:sprint4-dynamic','sathonate/fabric-worker:sprint5-recovery'):raise RuntimeError('Invalid qualification worker')
DYNAMIC=os.environ.get('SMP_DYNAMIC_LABEL','')
if DYNAMIC not in ('','a','b','c'):raise RuntimeError('Invalid dynamic qualification label')
RUN_LABEL=os.environ.get('SMP_QUALIFICATION_RUN_LABEL','')
if RUN_LABEL and not re.fullmatch('dynamic-[a-z0-9-]{1,48}',RUN_LABEL):raise RuntimeError('Invalid evidence label')
OUTPUT=ROOT/'.logs'/RUN_LABEL if RUN_LABEL else ROOT/'.logs'
OUTPUT.mkdir(parents=True,exist_ok=True)
RESULT=OUTPUT/(PREFIX+'-api-kind.json')
REPORT=OUTPUT/(PREFIX+'-qualification.json')
BASE=['kubectl','--kubeconfig',str(CONFIG),'--context','kind-'+NAME,'--request-timeout=20s']
report={'schema':'sathonate.'+PREFIX+'-qualification/v1','status':'BLOCKED','apiToFabric':'NOT VERIFIED','uiToFabric':'NOT_RERUN_'+PREFIX.upper() if PREFIX!='sprint1' else 'BLOCKED','cluster':NAME,'awsEnabled':False,'productionEnabled':False,'checks':{},'startedAt':time.time()}
created=False
CALICO=os.environ.get('SMP_QUALIFICATION_CALICO')=='1'
MULTI=os.environ.get('SMP_QUALIFICATION_MULTI_NODE')=='1'
if MULTI and (PREFIX!='sprint6' or not DYNAMIC or not CALICO):raise RuntimeError('Multi-node fixture requires explicit Sprint 6, dynamic configuration and Calico')
if PREFIX=='sprint6' and not MULTI:raise RuntimeError('Sprint 6 requires the multi-node fixture')

def run(args,data=None,timeout=240):
 if MULTI and Path(str(args[0])).name.lower() in ('docker','docker.exe','kind','kind.exe') and report.get('dockerEndpoint'):guard_engine()
 p=subprocess.run([str(x) for x in args],input=data,capture_output=True,text=True,encoding="utf-8",errors="replace",timeout=timeout)
 if p.returncode:raise RuntimeError('Qualification command failed: '+Path(str(args[0])).name+'; private diagnostics withheld')
 return p.stdout

def kub(*args,data=None):return run(BASE+list(args),data=data)
def apply(v):kub('apply','--server-side','--field-manager=sathonate-sprint1-fixture','-f','-',data=json.dumps(v))
def admission_denied(manifest,marker):
 p=subprocess.run(BASE+['create','--dry-run=server','-f','-'],input=json.dumps(manifest),capture_output=True,text=True,timeout=30)
 if p.returncode==0 or 'Forbidden' not in p.stderr or marker not in p.stderr:raise RuntimeError('Expected security admission denial not independently observed')

def image_nodes():
 return [NAME+'-control-plane']+([NAME+'-worker',NAME+'-worker2',NAME+'-worker3'] if MULTI else [])

def guard_engine():
 if MULTI and preflight.local_engine_endpoint()!=report.get('dockerEndpoint'):raise RuntimeError('Local Docker engine selection changed; command refused')

def guard():
 guard_engine()
 for node in image_nodes():
  if kub('get','node',node,'-o','jsonpath={.spec.providerID}')!='kind://docker/'+NAME+'/'+node:raise RuntimeError('Disposable node identity mismatch')

def cleanup_cluster():
 guard_engine()
 if MULTI:
  owned=[]
  for node in run([KIND,'get','nodes','--name',NAME]).splitlines():
   info=json.loads(run(['docker','inspect','--format','{"id":{{json .Id}},"labels":{{json .Config.Labels}},"paused":{{json .State.Paused}}}',node],timeout=20))
   if node not in image_nodes() or info['labels'].get('io.x-k8s.kind.cluster')!=NAME or not re.fullmatch('[a-f0-9]{64}',info['id']):raise RuntimeError('Partial fixture cleanup ownership mismatch')
   owned.append(info)
  # Validate the complete inventory before any mutation. Unpause even after a
  # child test timeout, then delete only this exact disposable cluster.
  for info in owned:
   if info['paused']:run(['docker','unpause',info['id']],timeout=20)
 else:guard()
 run([KIND,'delete','cluster','--name',NAME])

def load(image):
 if image=='sathonate/controller-candidate:atomic':
  run([KIND,'load','docker-image',image,'--name',NAME])
  for node in image_nodes():
   rows=run(['docker','exec',node,'ctr','-n','k8s.io','images','list']).splitlines()
   row=next(line.split() for line in rows if line.split() and line.split()[0]=='docker.io/'+image)
   pinned=image.split(':')[0]+'@'+row[2]
   run(['docker','exec',node,'ctr','-n','k8s.io','images','tag','--force','docker.io/'+image,'docker.io/'+pinned])
  return pinned,image+'@'+row[2]
 identity=run(['docker','image','inspect','--format','{{.Id}}',image]).strip()
 if not re.fullmatch('sha256:[a-f0-9]{64}',identity):raise RuntimeError('Public image identity invalid')
 archive=CACHE/('sprint1-public-'+identity.split(':')[1]+'.tar');receipt=archive.with_suffix('.sha256')
 def digest(path):
  with path.open('rb') as stream:return hashlib.file_digest(stream,'sha256').hexdigest()
 if not archive.exists() or not receipt.exists() or digest(archive)!=receipt.read_text():
  original=archive.with_suffix('.original.tar');run(['docker','image','save','--platform','linux/amd64','-o',original,image])
  try:
   with tarfile.open(original) as source,tarfile.open(archive,'w') as target:
    for member in source:
     if member.name not in ('index.json','oci-layout'):target.addfile(member,source.extractfile(member) if member.isfile() else None)
   receipt.write_text(digest(archive))
  finally:original.unlink(missing_ok=True)
 for node in image_nodes():
  with archive.open('rb') as stream:
   p=subprocess.run(['docker','exec','-i',node,'ctr','-n','k8s.io','images','import','--platform','linux/amd64','-'],stdin=stream,capture_output=True,timeout=180)
   if p.returncode:raise RuntimeError('Public image import failed')
  lines=run(['docker','exec',node,'ctr','-n','k8s.io','images','list']).splitlines()
  canonical=image if image.startswith('quay.io/') else 'docker.io/'+image
  row=next(line.split() for line in lines if line.split() and line.split()[0]==canonical)
  if not re.fullmatch('sha256:[a-f0-9]{64}',row[2]):raise RuntimeError('Runtime digest missing')
  pinned=image.split(':')[0]+'@'+row[2]
  run(['docker','exec',node,'ctr','-n','k8s.io','images','tag','--force',canonical,pinned if image.startswith('quay.io/') else 'docker.io/'+pinned])
  # Bevel uses separate repository and version fields.
  run(['docker','exec',node,'ctr','-n','k8s.io','images','tag','--force',canonical,(image if image.startswith('quay.io/') else 'docker.io/'+image)+'@'+row[2]])
 return pinned,image+'@'+row[2]

def inspect(namespace,worker,api=None):
 api=api or {}
 guard();secret_list=json.loads(kub('-n',namespace,'get','secrets','-o','json'))['items'];private=[]
 def credentials(v):
  if isinstance(v,dict):
   for key,value in v.items():
    if key in ('pass','password','secret') and isinstance(value,str) and value:private.append(value)
    credentials(value)
  elif isinstance(v,list):
   for x in v:credentials(x)
 for item in secret_list:
  if item['metadata'].get('labels',{}).get('owner')=='helm':continue
  for value in item.get('data',{}).values():
   raw=base64.b64decode(value).decode(errors='replace')
   if 'PRIVATE KEY' in raw:private.append(raw)
   if re.fullmatch('[a-f0-9]{64}',raw):private.append(raw)
   try:credentials(json.loads(raw))
   except ValueError:pass
 known=private+[base64.b64encode(x.encode()).decode() for x in private]
 def clean(raw):
  return 'PRIVATE KEY' not in raw and not re.search(r'eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{16,}',raw) and not any(x and x in raw for x in known)
 public=kub('-n',namespace,'get','fabriccas,fabricpeers,fabricorderernodes,fabricchaincodes,configmaps,pods,deployments','-o','json')
 if not clean(public):raise RuntimeError('Credential in public resources')
 report['checks']['publicResourcesNoCredentials']='PASS'
 release_count=0;release_names=set()
 for item in secret_list:
  if item['metadata'].get('labels',{}).get('owner')!='helm':continue
  release=gzip.decompress(base64.b64decode(base64.b64decode(item['data']['release']))).decode()
  if not clean(release):raise RuntimeError('Credential in decoded Helm history')
  release_count+=1;release_names.add(item['metadata']['labels'].get('name',''))
 if not set(api.get('expectedReleases',['org1-ca','org2-ca','org1-peer-1','org2-peer-1','orderer-1','orderer-2','orderer-3'])).issubset(release_names):raise RuntimeError('Incomplete Helm release/history inspection')
 report['checks']['helmHistoryNoCredentials']='PASS'
 pods=json.loads(kub('-n',namespace,'get','pods','-o','json'))['items']
 log_count=0
 for pod in pods:
  for container in pod['spec']['containers']:
   logs=kub('-n',namespace,'logs',pod['metadata']['name'],'-c',container['name'])
   if not clean(logs):raise RuntimeError('Credential in runtime logs')
   log_count+=1
 if log_count<9:raise RuntimeError('Incomplete log inspection')
 report['checks']['runtimeLogsNoCredentials']='PASS'
 inspected_files=0
 source_names=run(['git','ls-files','--cached','--others','--exclude-standard','--full-name','-z']).split('\0')
 for path in [Path(run(['git','rev-parse','--show-toplevel']).strip())/name for name in source_names if name]+list(OUTPUT.glob(PREFIX+'-*.json'))+list(OUTPUT.glob(PREFIX+'-*.log')):
  if not path.is_file():continue
  raw=path.read_text(encoding='utf-8',errors='replace')
  if any(x and x in raw for x in known):raise RuntimeError('Credential in source or evidence; private diagnostics withheld')
  inspected_files+=1
 report['checks']['sourceAndEvidenceNoActualCredentials']='PASS'
 report['sourceEvidenceFilesInspected']=inspected_files
 foreign=NAME+'-foreign';apply({'apiVersion':'v1','kind':'Namespace','metadata':{'name':foreign}})
 apply({'apiVersion':'v1','kind':'Secret','metadata':{'name':'foreign-canary','namespace':foreign},'stringData':{'canary':'disposable-authorization-test'}})
 for account in ('secure-controller','secure-issuer'):
  response=subprocess.run(BASE+['--as=system:serviceaccount:'+namespace+':'+account,'-n',foreign,'get','secret','foreign-canary'],capture_output=True,text=True,encoding="utf-8",errors="replace",timeout=30)
  if response.returncode==0 or 'forbidden' not in response.stderr.lower():raise RuntimeError('Foreign Secret denial missing')
  for verb,resource in [('create','clusterrolebindings'),('delete','namespaces'),('list','secrets')]:
   args=BASE+['auth','can-i',verb,resource,'--as=system:serviceaccount:'+namespace+':'+account]
   if resource=='secrets':args+=['--all-namespaces']
   p=subprocess.run(args,capture_output=True,text=True,encoding="utf-8",errors="replace",timeout=30)
   if p.stdout.strip()!='no':raise RuntimeError('Unexpected controller/issuer cluster authority')
 report['checks']['controllerIssuerForeignAndClusterDenials']='PASS'
 kub('-n',foreign,'get','secret','foreign-canary','-o','name');report['checks']['foreignResourcePreserved']='PASS'
 # A tokenless temporary pod performs actual DNS/CA-verified TLS handshakes.
 apply({'apiVersion':'v1','kind':'Pod','metadata':{'name':'secure-tls-observer','namespace':namespace},'spec':{'automountServiceAccountToken':False,'restartPolicy':'Never','securityContext':{'runAsUser':10001,'runAsGroup':10001},'containers':[{'name':'observer','image':worker,'imagePullPolicy':'Never','command':['sleep','300'],'securityContext':{'allowPrivilegeEscalation':False,'capabilities':{'drop':['ALL']}}}]}})
 kub('-n',namespace,'wait','--for=condition=Ready','pod/secure-tls-observer','--timeout=90s')
 script="import json,ssl,socket,hashlib,sys;v=json.load(sys.stdin);c=ssl.create_default_context(cadata=v['root']);s=c.wrap_socket(socket.create_connection((v['host'],7051),timeout=5),server_hostname=v['host']);print(hashlib.sha256(s.getpeercert(binary_form=True)).hexdigest());s.close()"
 for peer in api.get('inspectionPeers',['org1-peer-1','org2-peer-1']):
  identity=json.loads(kub('-n',namespace,'get','secret',peer+'-tls-identity','-o','json'))['data']
  cert=base64.b64decode(identity['cert.pem']).decode();root=base64.b64decode(identity['root.pem']).decode();expected=hashlib.sha256(ssl.PEM_cert_to_DER_cert(cert)).hexdigest()
  actual=kub('-n',namespace,'exec','-i','secure-tls-observer','--','python','-c',script,data=json.dumps({'root':root,'host':peer+'.'+namespace})).strip()
  if actual!=expected:raise RuntimeError('Live certificate differs from issuer material')
 report['checks']['bothLivePeerCertificatesVerified']='PASS'
 negative_script="import json,ssl,socket,sys;v=json.load(sys.stdin);c=ssl.create_default_context(cadata=v.get('root'));\ntry:\n s=c.wrap_socket(socket.create_connection((v['host'],7051),timeout=5),server_hostname=v.get('verifyHost',v['host']));s.close();print('ACCEPTED')\nexcept ssl.SSLCertVerificationError:print('REJECTED')"
 for peer in api.get('inspectionPeers',['org1-peer-1','org2-peer-1']):
  public=json.loads(kub('-n',namespace,'get','secret',peer+'-tls-identity','-o','json'))['data']
  root=base64.b64decode(public['root.pem']).decode();host=peer+'.'+namespace
  for case in ({'host':host},{'host':host,'root':root,'verifyHost':'unowned.invalid'}):
   if kub('-n',namespace,'exec','-i','secure-tls-observer','--','python','-c',negative_script,data=json.dumps(case)).strip()!='REJECTED':raise RuntimeError('Peer TLS accepted an untrusted issuer or foreign hostname')
 report['checks']['peerTLSUntrustedIssuerAndHostnameRejected']='PASS'

 report['inspection']={'helmReleases':release_count,'logStreams':log_count,'knownPrivateValues':len(private)}

try:
 if MULTI:
  sys.path.insert(0,str(ROOT/'tools/sprint6'));import preflight
  report['dockerEndpoint']=preflight.local_engine_endpoint()
  capacity=preflight.metrics();report['capacity']={'metrics':capacity,**preflight.assess(capacity)}
  if report['capacity']['status']!='PASS_PREFLIGHT_ONLY':raise RuntimeError('Sprint 6 capacity reservation not met; no resources created')
 else:run(['docker','info','--format','{{.ServerVersion}}'])
 metadata=json.loads(run(['docker','inspect','smp-postgres']))[0];values=dict(x.split('=',1) for x in metadata['Config']['Env'] if '=' in x)
 password=run(['docker','exec','smp-postgres','cat','/run/secrets/postgres-password']).strip()
 from urllib.parse import quote
 database='postgres://'+quote(values['POSTGRES_USER'],safe='')+':'+quote(password,safe='')+'@127.0.0.1:'+metadata['NetworkSettings']['Ports']['5432/tcp'][0]['HostPort']+'/fabric_smp_tests?sslmode=disable'
 print('Sprint 1: creating disposable API qualification cluster',flush=True)
 cluster_args=[KIND,'create','cluster','--name',NAME,'--image','kindest/node:v1.34.0','--kubeconfig',CONFIG,'--wait','0s' if CALICO else '180s']
 kind_config=CACHE/(NAME+'-kind.json')
 if CALICO:
  kind_config.write_text(json.dumps({'kind':'Cluster','apiVersion':'kind.x-k8s.io/v1alpha4','networking':{'disableDefaultCNI':True,'podSubnet':'192.168.0.0/16'},'nodes':[{'role':'control-plane'}]+([{'role':'worker'} for _ in range(3)] if MULTI else [])}));cluster_args+=['--config',kind_config]
 if NAME in run([KIND,'get','clusters']).splitlines():raise RuntimeError('Disposable name already exists; no adoption permitted')
 # Multi-node partial creation is tracked for independently guarded teardown.
 created=MULTI
 run(cluster_args);created=True;guard()
 if CALICO:
  sys.path.insert(0,str(ROOT/'tools/sprint2'));import isolation
  isolation.install(run,kub,apply,load,KIND,NAME,CACHE,report)
  if MULTI:kub('wait','nodes','--all','--for=condition=Ready','--timeout=180s')
 kind_config.unlink(missing_ok=True)
 if os.name=='nt':run(['icacls',CONFIG,'/inheritance:r','/grant:r',run(['whoami']).strip()+':F'])
 source=ROOT.parent/'dependencies/bevel-operator-fabric/config/crd/bases'
 for plural in ('fabriccas','fabricpeers','fabricorderernodes','fabricchaincodes'):
  path=source/('hlf.kungfusoftware.es_'+plural+'.yaml')
  kub('apply','--server-side','-f',str(path))
 images={}
 for image in ('sathonate/controller-candidate:atomic','hyperledger/fabric-ca:1.5.15','hyperledger/fabric-peer:2.5.15','hyperledger/fabric-orderer:2.5.15','sathonate/smp-smoke:local',WORKER,*(['sathonate/smp-smoke:dynamic-v1','sathonate/smp-smoke:dynamic-v2'] if DYNAMIC else [])):
  print('Sprint 1: loading public image '+image,flush=True);images[image]=load(image)
 controller=images['sathonate/controller-candidate:atomic'][0]
 if controller!='sathonate/controller-candidate@sha256:427b352b20576258efdb59c9fc266ec01fe7b7f1419ce1e5d7ead488b10cba6c':raise RuntimeError('Controller differs from verified profile')
 runtime={'storageClass':'standard','capacity':'1Gi','smokeImage':images['sathonate/smp-smoke:local'][0],'images':{kind:images[image][1] for kind,image in [('FabricCA','hyperledger/fabric-ca:1.5.15'),('FabricPeer','hyperledger/fabric-peer:2.5.15'),('FabricOrdererNode','hyperledger/fabric-orderer:2.5.15')]},'brokerChannels':True,'nodeSelectors':{name:{'kubernetes.io/hostname':NAME+'-control-plane'} for name in ('orderer-1','orderer-2','orderer-3','org1-peer-1','org2-peer-1')}}
 if DYNAMIC:
  approved={}
  for version in ('1','2'):
   image='sathonate/smp-smoke:dynamic-v'+version
   code_sha=run(['docker','image','inspect','--format','{{index .Config.Labels "org.sathonate.code.sha256"}}',image]).strip()
   if not re.fullmatch('[a-f0-9]{64}',code_sha):raise RuntimeError('Approved executable digest missing')
   approved[images[image][0]]={'sha256':code_sha,'codeVersion':version+'.0'}
  runtime.update(dynamicEnabled=True,codeArtifacts=approved)
 if MULTI:runtime['nodeSelectors']={node:{'kubernetes.io/hostname':node} for node in image_nodes()[1:]}
 config={'kubeconfig':str(CONFIG),'clusterName':NAME,'workerImage':images[WORKER][0],'runtime':runtime}
 if MULTI:config['workerNodes']=image_nodes()[1:]
 RUNTIME.write_text(json.dumps(config))
 env=dict(os.environ,GOCACHE=str(CACHE/'go-build'),GOPATH=str(CACHE/'go'),GOPROXY='off',SATHONATE_TEST_DATABASE_URL=database,SMP_SECURE_API_E2E='1',SMP_SECURE_CONFIG=str(RUNTIME),SMP_SECURE_API_EVIDENCE=str(RESULT),TMP=str(CACHE/'resume-tmp'),TEMP=str(CACHE/'resume-tmp'))
 print('Sprint 1: executing real authenticated Go API deployment/restart/retry test',flush=True)
 with (OUTPUT/(PREFIX+'-api-kind.log')).open('w') as output:
  test=subprocess.run(['go','test','./internal/httpapi','-run','^TestSecureApplicationAPIOnDisposableKind$','-count=1','-v','-timeout','45m' if MULTI else '25m'],cwd=ROOT,env=env,stdout=output,stderr=subprocess.STDOUT)
 report['apiTestExitCode']=test.returncode
 if test.returncode:
  # Explicit bounded diagnostic hold only on this guarded disposable cluster.
  # The normal finally block always performs teardown after the deadline.
  if os.environ.get('SMP_DIAGNOSTIC_HOLD')=='1':
   print('Disposable failure inspection window: '+NAME,flush=True)
   until=time.monotonic()+300
   while time.monotonic()<until and not (CACHE/(NAME+'-cleanup-now')).exists():time.sleep(2)
   (CACHE/(NAME+'-cleanup-now')).unlink(missing_ok=True)
  raise RuntimeError('Real API acceptance failed; see redacted Go test result')
 api=json.loads(RESULT.read_text());report['apiToFabric']=api['apiToFabric'];report['apiEvidenceSHA256']=hashlib.sha256(RESULT.read_bytes()).hexdigest();report['images']={key:value[0] for key,value in images.items()}
 if MULTI:
  import worker_live,worker_failure
  recovery=worker_live.qualify(run,kub,guard,ROOT,CACHE,RUNTIME,env,api,OUTPUT,report)
  report['workerFailureRecovery']=recovery
  if not worker_failure.completed_case(recovery):raise RuntimeError('Bounded worker failure/recovery acceptance failed; inspect redacted checkpoint')
  report['checks']['workerFailureRecovery']='PASS'
 print('Sprint 1: inspecting actual runtime resources and credentials',flush=True);inspect(api['namespace'],config['workerImage'],api)
 if CALICO:
  selected=None
  if MULTI:
   peer=api['inspectionPeers'][0]
   pods=json.loads(kub('-n',api['namespace'],'get','pods','-l','release='+peer,'-o','json'))['items']
   nodes={p['spec'].get('nodeName') for p in pods if not p['metadata'].get('deletionTimestamp')}
   if len(nodes)!=1:raise RuntimeError('Cross-worker peer placement unavailable')
   selected=worker_failure.cross_probe_workers(config['workerNodes'],next(iter(nodes)))
  isolation.inspect(kub,apply,admission_denied,api['namespace'],config['workerImage'],report,api.get('inspectionPeers',['org1-peer-1'])[0],api.get('expectedWorkloads',api.get('expectedReleases')),probe_nodes=selected)
 retained=json.loads(run(['kubectl','--kubeconfig',str(CACHE/'smp-qualification.kubeconfig'),'--context','kind-smp-qualification','-n','smp-n-968a212068be7d07','get','deployments','-o','json']))['items']
 ready=len(retained)==8 and all(x['status'].get('readyReplicas')==x['spec'].get('replicas',1)==1 for x in retained)
 report['retainedNetwork']={'readOnly':True,'readyDeployments':sum(x['status'].get('readyReplicas',0)==1 for x in retained),'total':len(retained),'ready':ready}
 if not ready:raise RuntimeError('Retained read-only readiness did not meet 8/8')
 report['status']=PREFIX.upper()+'_LOCAL_SECURITY_PASS_EXTERNAL_GATES_BLOCKED' if PREFIX!='sprint1' else 'BACKEND_LOCAL_QUALIFICATION_PASS_UI_BLOCKED'
 if MULTI:
  if not worker_failure.completed_case(report.get('workerFailureRecovery',{})) or report['checks'].get('crossWorkerCalicoEnforcement')!='PASS':raise RuntimeError('Multi-node acceptance gates incomplete')
  report['status']='SPRINT6_BOUNDED_LOCAL_MULTI_NODE_PASS_EXTERNAL_GATES_BLOCKED'
except Exception as error:
 report['failureCategory']=str(error) if isinstance(error,RuntimeError) else type(error).__name__
 raise
finally:
 cleanup_error=None
 try:
  if created:
   cleanup_cluster();CONFIG.unlink(missing_ok=True);RUNTIME.unlink(missing_ok=True);(CACHE/(NAME+'-kind.json')).unlink(missing_ok=True);report['disposableClusterRemoved']=True
 except Exception as error:
  cleanup_error=error;report['disposableClusterRemoved']=False;report['cleanupFailureCategory']=type(error).__name__;report['status']='FAIL_CLEANUP'
 finally:
  report['finishedAt']=time.time();REPORT.write_text(json.dumps(report,indent=2)+'\n')
 if cleanup_error:raise RuntimeError('Disposable cleanup not verified; preserve exact run identity and inspect cleanup receipt')
