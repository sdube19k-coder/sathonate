"""Disposable orchestration/inspection only. Deployment originates in the Go HTTP API test.
No CA issuance, Fabric deployment or transaction is implemented by this script.
"""
import base64,gzip,hashlib,json,os,re,ssl,subprocess,sys,tarfile,time,uuid
from pathlib import Path
CONSOLE=Path(__file__).resolve().parents[1]
ROOT=Path(os.environ.get('SMP_BACKEND_ROOT',str(CONSOLE.parent/'fabric-control-plane'))).resolve()
LABEL=os.environ.get('SMP_UI_EVIDENCE_LABEL','')
if LABEL and not re.fullmatch('dynamic-browser-[a-z0-9-]{1,48}',LABEL):raise RuntimeError('Invalid browser evidence label')
LOG=CONSOLE/'.logs'/LABEL if LABEL else CONSOLE/'.logs'
LOG.mkdir(parents=True,exist_ok=True)
CACHE=Path(os.environ.get('SMP_CACHE_ROOT',str(ROOT.parent/'.cache'))).resolve()
KIND=Path(os.environ.get('SMP_KIND_PATH',str(ROOT.parent/'tools/kind.exe'))).resolve()
NAME='smp-sprint1-'+uuid.uuid4().hex[:8]
CONFIG=CACHE/(NAME+'.kubeconfig')
RUNTIME=CACHE/(NAME+'-runtime.json')
RESULT=LOG/'browser-result.json'
REPORT=LOG/'ui-qualification.json'
BASE=['kubectl','--kubeconfig',str(CONFIG),'--context','kind-'+NAME,'--request-timeout=20s']
report={'schema':'sathonate.sprint1-qualification/v1','status':'BLOCKED','apiToFabric':'NOT VERIFIED','uiToFabric':'BLOCKED','cluster':NAME,'awsEnabled':False,'productionEnabled':False,'checks':{},'startedAt':time.time()}
created=False
LIFECYCLE=os.environ.get('SMP_UI_LIFECYCLE')=='1'
DYNAMIC=os.environ.get('SMP_UI_DYNAMIC')=='1'
SPRINT5=os.environ.get('SMP_UI_SPRINT5')=='1'
CALICO=os.environ.get('SMP_UI_CALICO')=='1'
SPRINT7=os.environ.get('SMP_UI_SPRINT7')=='1'
cluster_attempted=False
db_scoped=False
kind_config=CACHE/(NAME+'-kind.json')
WORKER='sathonate/fabric-worker:sprint5-recovery' if SPRINT5 and os.environ.get('SMP_UI_SPRINT5_WORKER')=='1' else 'sathonate/fabric-worker:sprint4-dynamic' if DYNAMIC else ('sathonate/fabric-worker:sprint4-lifecycle' if LIFECYCLE else 'sathonate/fabric-worker:sprint3')

def run(args,data=None,timeout=240):
 p=subprocess.run([str(x) for x in args],input=data,capture_output=True,text=True,encoding="utf-8",errors="replace",timeout=timeout)
 if p.returncode:raise RuntimeError('Qualification command failed: '+Path(str(args[0])).name+'; private diagnostics withheld')
 return p.stdout

def kub(*args,data=None):return run(BASE+list(args),data=data)
def apply(v):kub('apply','--server-side','--field-manager=sathonate-sprint1-fixture','-f','-',data=json.dumps(v))
def admission_denied(manifest,marker):
 p=subprocess.run(BASE+['create','--dry-run=server','-f','-'],input=json.dumps(manifest),capture_output=True,text=True,timeout=30)
 if p.returncode==0 or 'Forbidden' not in p.stderr or marker not in p.stderr:raise RuntimeError('Expected security admission denial not independently observed')

def guard():
 if kub('get','node',NAME+'-control-plane','-o','jsonpath={.spec.providerID}')!='kind://docker/'+NAME+'/'+NAME+'-control-plane':raise RuntimeError('Disposable identity mismatch')
def load(image):
 canonical=image if '.' in image.split('/')[0] else 'docker.io/'+image
 if image=='sathonate/controller-candidate:atomic':
  run([KIND,'load','docker-image',image,'--name',NAME])
  rows=run(['docker','exec',NAME+'-control-plane','ctr','-n','k8s.io','images','list']).splitlines()
  row=next(line.split() for line in rows if line.split() and line.split()[0]==canonical)
  pinned=image.split(':')[0]+'@'+row[2]
  run(['docker','exec',NAME+'-control-plane','ctr','-n','k8s.io','images','tag','--force',canonical, pinned if '.' in pinned.split('/')[0] else 'docker.io/'+pinned])
  return pinned,image+'@'+row[2]
 identity=run(['docker','image','inspect','--format','{{.Id}}',image]).strip()
 if not re.fullmatch('sha256:[a-f0-9]{64}',identity):raise RuntimeError('Public image identity invalid')
 archive=CACHE/('sprint1-public-'+identity.split(':')[1]+'.tar');receipt=archive.with_suffix('.sha256')
 digest=lambda p:hashlib.file_digest(p.open('rb'),'sha256').hexdigest()
 if not archive.exists() or not receipt.exists() or digest(archive)!=receipt.read_text():
  original=archive.with_suffix('.original.tar');run(['docker','image','save','--platform','linux/amd64','-o',original,image])
  try:
   with tarfile.open(original) as source,tarfile.open(archive,'w') as target:
    for member in source:
     if member.name not in ('index.json','oci-layout'):target.addfile(member,source.extractfile(member) if member.isfile() else None)
   receipt.write_text(digest(archive))
  finally:original.unlink(missing_ok=True)
 with archive.open('rb') as stream:
  p=subprocess.run(['docker','exec','-i',NAME+'-control-plane','ctr','-n','k8s.io','images','import','--platform','linux/amd64','-'],stdin=stream,capture_output=True,timeout=180)
  if p.returncode:raise RuntimeError('Public image import failed')
 lines=run(['docker','exec',NAME+'-control-plane','ctr','-n','k8s.io','images','list']).splitlines()
 row=next(line.split() for line in lines if line.split() and line.split()[0]==canonical)
 if not re.fullmatch('sha256:[a-f0-9]{64}',row[2]):raise RuntimeError('Runtime digest missing')
 pinned=image.split(':')[0]+'@'+row[2]
 run(['docker','exec',NAME+'-control-plane','ctr','-n','k8s.io','images','tag','--force',canonical, pinned if '.' in pinned.split('/')[0] else 'docker.io/'+pinned])
 # Bevel uses separate repository and version fields.
 run(['docker','exec',NAME+'-control-plane','ctr','-n','k8s.io','images','tag','--force',canonical,canonical+'@'+row[2]])
 return pinned,image+'@'+row[2]

def inspect(namespace,worker,additional_private=(),runtime_result=None):
 runtime_result=runtime_result or {}
 guard();secret_list=json.loads(kub('-n',namespace,'get','secrets','-o','json'))['items'];private=list(additional_private)
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
  return 'PRIVATE KEY' not in raw and not any(x and x in raw for x in known)
 public=kub('-n',namespace,'get','fabriccas,fabricpeers,fabricorderernodes,fabricchaincodes,configmaps,pods,deployments','-o','json')
 if not clean(public):raise RuntimeError('Credential in public resources')
 report['checks']['publicResourcesNoCredentials']='PASS'
 release_count=0;release_names=set()
 for item in secret_list:
  if item['metadata'].get('labels',{}).get('owner')!='helm':continue
  release=gzip.decompress(base64.b64decode(base64.b64decode(item['data']['release']))).decode()
  if not clean(release):raise RuntimeError('Credential in decoded Helm history')
  release_count+=1;release_names.add(item['metadata']['labels'].get('name',''))
 if not set(runtime_result.get('expectedReleases',['org1-ca','org2-ca','org1-peer-1','org2-peer-1','orderer-1','orderer-2','orderer-3'])).issubset(release_names):raise RuntimeError('Incomplete Helm release/history inspection')
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
 source_names=run(['git','-C',str(CONSOLE),'ls-files','--cached','--others','--exclude-standard','--full-name','-z']).split('\0')
 for path in [Path(run(['git','-C',str(CONSOLE),'rev-parse','--show-toplevel']).strip())/name for name in source_names if name]+list(LOG.glob('*.json'))+list(LOG.glob('*.log'))+list((CONSOLE/'dist').rglob('*')):
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
 for peer in runtime_result.get('inspectionPeers',('org1-peer-1','org2-peer-1','org1-peer-2') if LIFECYCLE else ('org1-peer-1','org2-peer-1')):
  identity=json.loads(kub('-n',namespace,'get','secret',peer+'-tls-identity','-o','json'))['data']
  cert=base64.b64decode(identity['cert.pem']).decode();root=base64.b64decode(identity['root.pem']).decode();expected=hashlib.sha256(ssl.PEM_cert_to_DER_cert(cert)).hexdigest()
  actual=kub('-n',namespace,'exec','-i','secure-tls-observer','--','python','-c',script,data=json.dumps({'root':root,'host':peer+'.'+namespace})).strip()
  if actual!=expected:raise RuntimeError('Live certificate differs from issuer material')
  for mode in ('wrongIssuer','wrongHostname'):
   negative="import json,ssl,socket,sys;v=json.load(sys.stdin);c=ssl.create_default_context() if v['mode']=='wrongIssuer' else ssl.create_default_context(cadata=v['root']);\ntry:\n s=c.wrap_socket(socket.create_connection((v['host'],7051),timeout=5),server_hostname='invalid.sathonate.invalid' if v['mode']=='wrongHostname' else v['host']);s.close();print('ACCEPTED')\nexcept ssl.SSLCertVerificationError:print('REJECTED')"
   outcome=kub('-n',namespace,'exec','-i','secure-tls-observer','--','python','-c',negative,data=json.dumps({'root':root,'host':peer+'.'+namespace,'mode':mode})).strip()
   if outcome!='REJECTED':raise RuntimeError('Live TLS negative trust verification failed')

 report['checks']['bothLivePeerCertificatesVerified']='PASS'
 report['checks']['liveWrongIssuerAndHostnameRejected']='PASS'
 report['inspection']={'helmReleases':release_count,'logStreams':log_count,'knownPrivateValues':len(private)}

import secrets,socket,urllib.request
from datetime import datetime,timezone,timedelta
processes=[];handles=[];grants_path=CACHE/(NAME+'-grants.json')
def start(args,env,log):
 handle=(LOG/log).open('w');handles.append(handle)
 process=subprocess.Popen(args,cwd=CONSOLE,env=env,stdout=handle,stderr=subprocess.STDOUT,creationflags=subprocess.CREATE_NO_WINDOW if os.name=='nt' else 0);processes.append(process);return process
def wait_url(url):
 for _ in range(90):
  try:
   with urllib.request.urlopen(url,timeout=2):return
  except Exception:time.sleep(1)
 raise RuntimeError('Local application listener unavailable')
try:
 for port in (8080,5173,5174):
  with socket.socket() as probe:
   probe.settimeout(0.5)
   if probe.connect_ex(('127.0.0.1',port))==0:raise RuntimeError('Required local fixture port '+str(port)+' is already in use; no existing process was stopped')
 if SPRINT7:
  sys.path.insert(0,str(ROOT/'tools/sprint7'))
  from qualify import preflight
  report['resourcePreflight']=preflight()
  if report['resourcePreflight']['status']!='PASS_PREFLIGHT_ONLY':raise RuntimeError('Sprint 7 capacity preflight blocked before Docker or Kubernetes mutation')
  if not (DYNAMIC and SPRINT5 and CALICO and os.environ.get('SMP_UI_SPRINT5_OPERATIONS')=='1' and not any(os.environ.get(name)=='1' for name in ('SMP_UI_SPRINT5_MINIMAL','SMP_UI_SPRINT5_OPERATIONS_ONLY','SMP_UI_SPRINT5_COLD_MINIMAL','SMP_UI_SPRINT5_DELETION_ONLY'))):raise RuntimeError('Sprint 7 requires the full dynamic and recovery fixture')
 run(['docker','info','--format','{{.ServerVersion}}'])
 metadata=json.loads(run(['docker','inspect','smp-postgres']))[0];values=dict(x.split('=',1) for x in metadata['Config']['Env'] if '=' in x)
 password=run(['docker','exec','smp-postgres','cat','/run/secrets/postgres-password']).strip()
 from urllib.parse import quote
 database='postgres://'+quote(values['POSTGRES_USER'],safe='')+':'+quote(password,safe='')+'@127.0.0.1:'+metadata['NetworkSettings']['Ports']['5432/tcp'][0]['HostPort']+'/fabric_smp_tests?sslmode=disable'
 if SPRINT7:
  api_binary=Path(os.environ['SMP_UI_API_BINARY']).resolve()
  expected_api=os.environ.get('SMP_UI_API_SHA256','')
  if api_binary.parent!=LOG.resolve() or not re.fullmatch('[a-f0-9]{64}',expected_api) or hashlib.sha256(api_binary.read_bytes()).hexdigest()!=expected_api:raise RuntimeError('Current-source API fixture identity mismatch')
  migration=subprocess.run([api_binary,'-mode','local-development','-migrate'],env=dict(os.environ,DATABASE_URL=database),capture_output=True,timeout=90)
  if migration.returncode:raise RuntimeError('Dedicated PostgreSQL test schema migration failed')
  if run(['docker','ps','-aq','--filter','label=io.x-k8s.kind.cluster='+NAME]).strip():raise RuntimeError('Disposable name already exists')
  report['resourcePreflight']=preflight()
  if report['resourcePreflight']['status']!='PASS_PREFLIGHT_ONLY':raise RuntimeError('Sprint 7 capacity changed before kind creation')
 print('UI qualification: creating fresh disposable kind cluster',flush=True)
 cluster_args=[KIND,'create','cluster','--name',NAME,'--image','kindest/node:v1.34.0','--kubeconfig',CONFIG,'--wait','0s' if CALICO else '180s']
 if CALICO:
  kind_config.write_text(json.dumps({'kind':'Cluster','apiVersion':'kind.x-k8s.io/v1alpha4','networking':{'disableDefaultCNI':True,'podSubnet':'192.168.0.0/16'},'nodes':[{'role':'control-plane'}]}));cluster_args+=['--config',kind_config]
 cluster_attempted=True
 run(cluster_args);created=True;guard()
 if CALICO:
  sys.path.insert(0,str(ROOT/'tools/sprint2'));import isolation
  isolation.install(run,kub,apply,load,KIND,NAME,CACHE,report)
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
  if SPRINT7:runtime['nodeSelectors']['approved-local']={'kubernetes.io/hostname':NAME+'-control-plane'}
  approved={}
  for version in ('1','2'):
   image='sathonate/smp-smoke:dynamic-v'+version
   code_sha=run(['docker','image','inspect','--format','{{index .Config.Labels "org.sathonate.code.sha256"}}',image]).strip()
   if not re.fullmatch('[a-f0-9]{64}',code_sha):raise RuntimeError('Approved executable digest missing')
   approved[images[image][0]]={'sha256':code_sha,'codeVersion':version+'.0'}
  runtime.update(dynamicEnabled=True,codeArtifacts=approved)
 config={'kubeconfig':str(CONFIG),'clusterName':NAME,'workerImage':images[WORKER][0],'runtime':runtime};RUNTIME.write_text(json.dumps(config))

 # The unchanged qualified worker is retained; the API must be a freshly built
 # binary with its SHA supplied by the local verification command.
 baseline=json.loads((ROOT/('.logs/sprint5-worker-build.json' if WORKER.endswith(':sprint5-recovery') else '.logs/sprint4-dynamic-build.json' if DYNAMIC else ('.logs/sprint4-lifecycle-worker-build.json' if LIFECYCLE else '.logs/sprint3-worker-build.json'))).read_text())
 if run(['docker','image','inspect','--format','{{.Id}}',WORKER]).strip()!=baseline['workerImageID']:raise RuntimeError('Qualified worker image differs')
 api_binary=api_binary if SPRINT7 else ROOT/('bin/fabric-api-sprint5.exe' if SPRINT5 else ('bin/fabric-api-sprint4-dynamic.exe' if DYNAMIC else ('bin/fabric-api-sprint4-lifecycle.exe' if LIFECYCLE else 'bin/fabric-api-sprint4.exe')))
 expected_api=os.environ.get('SMP_UI_API_SHA256','')
 if not re.fullmatch('[a-f0-9]{64}',expected_api):raise RuntimeError('Fresh API binary SHA256 required')
 if hashlib.sha256(api_binary.read_bytes()).hexdigest()!=expected_api:raise RuntimeError('Verified API binary differs')
 report['provenance']={'backendCommit':run(['git','-C',ROOT,'rev-parse','HEAD']).strip(),'apiBinarySHA256':expected_api,'workerImageID':baseline['workerImageID'],'backendDiffSHA256':hashlib.sha256(run(['git','-C',ROOT,'diff','HEAD']).encode()).hexdigest()}
 now=datetime.now(timezone.utc);tokens={key:secrets.token_hex(32) for key in ('primary','foreignTenant','foreignProject','noDeploy','expired','invalid','administrator')}
 grants=[]
 for key,tenant,project,permissions,expiry in [('primary',NAME,'project-a',['read','write','deploy','retry','cancel'],3600),('foreignTenant',NAME+'-other','project-a',['read','write','deploy','retry'],3600),('foreignProject',NAME,'project-b',['read','write','deploy','retry'],3600),('noDeploy',NAME,'project-a',['read','write'],3600),('expired',NAME,'project-a',['read'],12),('administrator',NAME,'project-a',['read','write','cleanup'],3600)]:
  grants.append({'tokenSHA256':hashlib.sha256(tokens[key].encode()).hexdigest(),'developer':'browser-qualification','tenant':tenant,'project':project,'permissions':permissions,'expiresAt':(now+timedelta(seconds=expiry)).isoformat()})
 grants_path.write_text(json.dumps(grants))
 if os.name=='nt':run(['icacls',grants_path,'/inheritance:r','/grant:r',run(['whoami']).strip()+':F'])
 env=dict(os.environ,DATABASE_URL=database,VITE_LOCAL_AUTH_ENABLED='true',VITE_APP_ENV='local-development',DEBUG='',PWDEBUG='0')
 db_scoped=SPRINT7
 api_process=start([api_binary,'-mode','local-development','-deployment-mode','secure-local','-local-token-grants',grants_path,'-secure-local-config',RUNTIME,'-addr','127.0.0.1:8080'],env,'api-server.log')
 vite=CONSOLE/'node_modules/vite/bin/vite.js'
 frontend_env={key:value for key,value in env.items() if key!='DATABASE_URL'}
 start(['node',vite,'--host','127.0.0.1','--port','5173','--strictPort'],frontend_env,'console-server.log')
 start(['node',vite,'preview','--host','127.0.0.1','--port','5174','--strictPort'],frontend_env,'preview-server.log')
 wait_url('http://127.0.0.1:8080/api/v1/auth/config');wait_url('http://127.0.0.1:5173/');wait_url('http://127.0.0.1:5174/')
 print('UI qualification: actual browser authentication, deployment, refresh, fault/retry and transactions',flush=True)
 with (LOG/'browser.log').open('w') as log:
  test=subprocess.run(['node','tools/browser-dynamic.mjs' if DYNAMIC else 'tools/browser-acceptance.mjs'],cwd=CONSOLE,input=json.dumps({'tokens':tokens,'tenant':NAME,'project':'project-a','cluster':NAME,'kubeconfig':str(CONFIG),'result':str(RESULT),'logDirectory':str(LOG),'lifecycle':LIFECYCLE,'sprint7':SPRINT7,'sprint5':SPRINT5,'sprint5Minimal':os.environ.get('SMP_UI_SPRINT5_MINIMAL')=='1','sprint5OperationsOnly':os.environ.get('SMP_UI_SPRINT5_OPERATIONS_ONLY')=='1','runtime':runtime,'design':json.loads((ROOT/'examples/dynamic/a.json').read_text()) if DYNAMIC else None}),text=True,encoding='utf-8',stdout=log,stderr=subprocess.STDOUT,timeout=2700 if SPRINT7 else 1800)
 report['browserExitCode']=test.returncode
 if test.returncode:raise RuntimeError('Browser acceptance failed; see redacted browser log')
 result=json.loads(RESULT.read_text());report['uiToFabric']='PASS';report['apiToFabric']='PASS';report['browser']=result;report['images']={k:v[0] for k,v in images.items()}
 inspect(result['namespace'],config['workerImage'],list(tokens.values())+[password],result)
 if CALICO:
  isolation.inspect(kub,apply,admission_denied,result['namespace'],config['workerImage'],report,result.get('inspectionPeers',['org1-peer-1'])[0],result.get('expectedWorkloads'))
 if SPRINT5 and os.environ.get('SMP_UI_SPRINT5_OPERATIONS')=='1':
  import importlib.util
  module_spec=importlib.util.spec_from_file_location('sprint5_acceptance',ROOT/'tools/sprint5/acceptance.py');operations_acceptance=importlib.util.module_from_spec(module_spec);module_spec.loader.exec_module(operations_acceptance)
  def restart_api():
   # A deliberately expired negative-test grant must not poison a later safe
   # startup. Remove expired entries; never extend expiry or change permissions.
   auth_spec=importlib.util.spec_from_file_location('fixture_auth',ROOT/'tools/sprint5/fixture_auth.py');fixture_auth=importlib.util.module_from_spec(auth_spec);auth_spec.loader.exec_module(fixture_auth)
   active=fixture_auth.active_grants(json.loads(grants_path.read_text()),datetime.now(timezone.utc))
   grants_path.write_text(json.dumps(active))
   return start([api_binary,'-mode','local-development','-deployment-mode','secure-local','-local-token-grants',grants_path,'-secure-local-config',RUNTIME,'-addr','127.0.0.1:8080'],env,'api-server-after-cold.log')
  operations_acceptance.qualify(run,kub,apply,guard,report,result,ROOT,CACHE,RUNTIME,env,tokens,NAME,api_process,restart_api,LOG)
 retained=json.loads(run(['kubectl','--kubeconfig',str(CACHE/'smp-qualification.kubeconfig'),'--context','kind-smp-qualification','-n','smp-n-968a212068be7d07','get','deployments','-o','json']))['items']
 ready=len(retained)==8 and all(x['status'].get('readyReplicas')==x['spec'].get('replicas',1)==1 for x in retained)
 report['retainedNetwork']={'readOnly':True,'ready':ready,'readyDeployments':sum(x['status'].get('readyReplicas',0)==1 for x in retained),'total':len(retained)}
 if not ready:raise RuntimeError('Retained network did not meet 8/8 read-only readiness')
 report['status']='PASS'
except Exception as error:
 if SPRINT7 and cluster_attempted:report['status']='FAIL'
 report['failureCategory']=str(error) if isinstance(error,RuntimeError) else type(error).__name__
 raise
finally:
 for process in reversed(processes):
  process.terminate()
  try:process.wait(timeout=15)
  except subprocess.TimeoutExpired:process.kill();process.wait()
 for handle in handles:handle.close()
 grants_path.unlink(missing_ok=True)
 cleanup_failed=False
 try:
  if SPRINT7 and cluster_attempted:
   ids=run(['docker','ps','-aq','--filter','label=io.x-k8s.kind.cluster='+NAME]).split()
   if len(ids)>1:raise RuntimeError('Unexpected disposable node inventory; cleanup refused')
   if ids:
    fields=run(['docker','inspect','--format','{{.Name}}|{{index .Config.Labels "io.x-k8s.kind.cluster"}}',ids[0]]).strip()
    if fields!='/'+NAME+'-control-plane|'+NAME:raise RuntimeError('Disposable Docker ownership mismatch; cleanup refused')
    run(['docker','rm','-f','-v',ids[0]])
   if run(['docker','ps','-aq','--filter','label=io.x-k8s.kind.cluster='+NAME]).strip():raise RuntimeError('Disposable cleanup not verified')
   report['disposableClusterRemoved']=True
  elif created:
   guard();run([KIND,'delete','cluster','--name',NAME]);report['disposableClusterRemoved']=True
  if report.get('disposableClusterRemoved'):
   CONFIG.unlink(missing_ok=True);RUNTIME.unlink(missing_ok=True);kind_config.unlink(missing_ok=True)
 except Exception:
  report['clusterCleanup']='FAIL';cleanup_failed=True
 try:
  if db_scoped:
   scopes=["'lp-"+hashlib.sha256(json.dumps([tenant,project],separators=(',',':')).encode()).hexdigest()[:48]+"'" for tenant,project in ((NAME,'project-a'),(NAME+'-other','project-a'),(NAME,'project-b'))]
   ids=','.join(scopes)
   tables=['sathonate_deployments','sathonate_operations','sathonate_network_revisions','sathonate_networks','sathonate_maintenance']
   sql='BEGIN;'+''.join(f'DELETE FROM {table} WHERE tenant_id IN ({ids});' for table in tables)+f'DELETE FROM public.sathonate_audit_events WHERE scope IN ({ids});COMMIT;'
   run(['docker','exec','-i','smp-postgres','psql','-X','-q','-v','ON_ERROR_STOP=1','-U',values['POSTGRES_USER'],'-d','fabric_smp_tests'],data=sql)
   report['fixtureDatabaseScopeRemoved']=True
 except Exception:
  report['fixtureDatabaseScopeRemoved']=False;cleanup_failed=True
 if cleanup_failed:report['status']='FAIL'
 report['finishedAt']=time.time();REPORT.write_text(json.dumps(report,indent=2)+'\n')
