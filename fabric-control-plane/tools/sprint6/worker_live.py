"""Actual disposable-kind adapter for bounded worker pause/rescheduling recovery.
Never accepts a retained target, never force-deletes stateful pods or changes PVs.
"""
import base64,hashlib,json,os,re,secrets,socket,subprocess,threading,time,urllib.request,urllib.error
from datetime import datetime,timezone,timedelta
from urllib.parse import urlsplit,unquote
from worker_failure import Failure,require,candidates,pv_host,unchanged_storage,run_case

def require_disposable_database(connection):
 try:value=urlsplit(connection)
 except ValueError:raise Failure('dedicated-loopback-database-required') from None
 require(value.scheme in ('postgres','postgresql') and value.hostname in ('127.0.0.1','localhost','::1') and value.path=='/fabric_smp_tests' and not value.fragment,'dedicated-loopback-database-required')

def listener_owned(port,pid):
 require(type(port) is int and 0<port<65536 and type(pid) is int and pid>0,'listener-identity-input')
 require(os.name=='nt','listener-ownership-platform-unsupported')
 # Query only this loopback listener before sending any bearer token. A
 # bind/close/start race must never disclose a token to another local server.
 command='Get-NetTCPConnection -LocalAddress 127.0.0.1 -LocalPort '+str(port)+' -State Listen -ErrorAction SilentlyContinue | Select-Object -ExpandProperty OwningProcess'
 result=subprocess.run(['powershell.exe','-NoProfile','-NonInteractive','-Command',command],capture_output=True,text=True,timeout=15)
 owners={int(line.strip()) for line in result.stdout.splitlines() if line.strip().isdigit()}
 return result.returncode==0 and owners=={pid}

def redact_log(lines,output,values,mark):
 masked=False
 for line in lines:
  if masked or 'PRIVATE KEY' in line:
   mark();masked='-----END' not in line and (masked or '-----BEGIN' in line);output.write('[REDACTED_PRIVATE_MATERIAL]\n');output.flush();continue
  clean=line
  for value in values:
   if value and value in clean:mark();clean=clean.replace(value,'[REDACTED]')
  output.write(clean);output.flush()

class API:
 def __init__(self,port,tokens,private_values=()):self.base='http://127.0.0.1:'+str(port)+'/api/v1';self.tokens=tokens;self.private_values=list(tokens.values())+list(private_values)
 def request(self,path,role='primary',method='GET',body=None,headers=None):
  token=self.tokens.get(role,'invalid')
  req=urllib.request.Request(self.base+path,data=None if body is None else json.dumps(body).encode(),method=method,headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if role is not None else {}),**(headers or {})})
  try:
   with urllib.request.urlopen(req,timeout=30) as response:status,raw=response.status,response.read(2**20)
  except urllib.error.HTTPError as error:
   try:status,raw=error.code,error.read(2**20)
   finally:error.close()
  require(all(value.encode() not in raw for value in self.private_values if value) and b'PRIVATE KEY' not in raw,'credential-in-api-response')
  try:value=json.loads(raw)
  except ValueError:
   require(status>=400,'api-response-framing');value={'error':{'code':'http-'+str(status)}}
  return status,value

def valid_transaction(raw,txid):
 require(bool(re.fullmatch('[a-f0-9]{64}',txid)) and txid.encode() in raw,'prior-transaction-absent')
 pos=0;code=0
 def number():
  nonlocal pos
  value=0;shift=0
  while pos<len(raw) and shift<70:
   byte=raw[pos];pos+=1;value|=(byte&127)<<shift
   if not byte&128:return value
   shift+=7
  raise Failure('transaction-response-malformed')
 while pos<len(raw):
  tag=number();wire=tag&7
  require(tag>>3>0,'transaction-response-malformed')
  if wire==0:
   value=number()
   if tag>>3==2:code=value
  elif wire==2:
   size=number();pos+=size
  elif wire==1:pos+=8
  elif wire==5:pos+=4
  else:raise Failure('transaction-response-malformed')
  require(pos<=len(raw),'transaction-response-truncated')
 require(code==0,'prior-transaction-invalid')

class Live:
 def __init__(self,run,kub,guard,root,cache,runtime,environment,initial,output):
  self.run,self.kub,self.parent_guard=run,kub,guard
  self.root,self.cache,self.runtime,self.env,self.initial,self.output=root,cache,runtime,environment,initial,output
  require_disposable_database(environment.get('SATHONATE_TEST_DATABASE_URL',''))
  self.config=json.loads(runtime.read_text());self.config_hash=hashlib.sha256(runtime.read_bytes()).hexdigest()
  self.cluster=self.config['clusterName'];self.ns=initial['namespace'];self.operation=initial['operationId']
  require(re.fullmatch('smp-sprint1-[a-f0-9]{8}',self.cluster) is not None,'disposable-cluster-name')
  self.workers=[self.cluster+'-worker',self.cluster+'-worker2',self.cluster+'-worker3']
  require(len(self.config.get('workerNodes',[]))==3 and set(self.config['workerNodes'])==set(self.workers),'owned-three-worker-inventory')
  self.created=[];self.process=None;self.log_handle=None;self.log_thread=None;self.log_exposure=False;self.log_stream_failed=False;self.private=None;self.grant_created=False;self.tokens={};self.target_id=None;self.namespace_uid=None;self.paused_at=None;self.resume_lock=threading.RLock();self.watchdog=None;self.watchdog_fired=False;self.watchdog_error=False
 def guard(self):
  self.parent_guard();require(hashlib.sha256(self.runtime.read_bytes()).hexdigest()==self.config_hash,'target-config-changed')
  obj=json.loads(self.kub('get','namespace',self.ns,'-o','json'));m=obj['metadata'];labels=m.get('labels',{})
  storage='lp-'+hashlib.sha256(json.dumps(['sprint1-'+self.cluster.removeprefix('smp-sprint1-'),'project-a'],separators=(',',':')).encode()).hexdigest()[:48]
  require(labels.get('fabric.sathonate.io/tenant')==storage and labels.get('fabric.sathonate.io/project')=='token-bound-project' and labels.get('fabric.sathonate.io/profile')=='smp-secure-local-v1','namespace-scope')
  network=labels.get('fabric.sathonate.io/network','')
  expected='smp-s-'+hashlib.sha256(json.dumps([storage,'token-bound-project',network],separators=(',',':')).encode()).hexdigest()[:32]
  require(expected==self.ns and network,'namespace-network-binding')
  if self.namespace_uid is None:self.namespace_uid=m['uid']
  require(m['uid']==self.namespace_uid,'namespace-replaced')
  require(not labels.get('fabric.sathonate.io/retired') and not labels.get('fabric.sathonate.io/cold-maintenance'),'namespace-fenced')
  self.network_id=network
 def wait(self,check,seconds,code,outage=False):
  end=time.monotonic()+seconds
  while time.monotonic()<end:
   if outage:require(time.monotonic()-self.paused_at<180,'bounded-pause-deadline')
   value=check()
   if value:return value
   time.sleep(1)
  raise Failure(code)
 def create(self,kind,name,spec):
  self.guard();body={'apiVersion':'apps/v1' if kind=='Deployment' else 'v1','kind':kind,'metadata':{'name':name,'namespace':self.ns,'labels':{'sathonate.io/sprint6-fixture':self.cluster}},'spec':spec}
  ref={'kind':kind,'name':name,'uid':None};self.created.append(ref)
  raw=json.loads(self.kub('create','-f','-','-o','json',data=json.dumps(body)))
  require(raw['metadata']['namespace']==self.ns and raw['metadata']['uid'],'fixture-resource-identity')
  ref['uid']=raw['metadata']['uid'];return ref
 def delete(self,ref):
  self.guard();resource='deployments' if ref['kind']=='Deployment' else 'pods'
  raw=self.kub('-n',self.ns,'get',resource,ref['name'],'--ignore-not-found','-o','json')
  if not raw.strip():return
  require(json.loads(raw)['metadata']['uid']==ref['uid'],'cleanup-replacement-refused')
  base='/apis/apps/v1' if resource=='deployments' else '/api/v1'
  path=base+'/namespaces/'+self.ns+'/'+resource+'/'+ref['name']
  self.kub('delete','--raw',path,'-f','-',data=json.dumps({'apiVersion':'v1','kind':'DeleteOptions','propagationPolicy':'Foreground','preconditions':{'uid':ref['uid']}}))
  self.kub('-n',self.ns,'wait','--for=delete',resource+'/'+ref['name'],'--timeout=90s')
 def node(self,name):return json.loads(self.kub('get','node',name,'-o','json'))
 def node_ready(self,name):return any(c.get('type')=='Ready' and c.get('status')=='True' for c in self.node(name).get('status',{}).get('conditions',[]))
 def pods(self):return json.loads(self.kub('-n',self.ns,'get','pods','-o','json'))['items']
 def ready(self,pod):return any(c.get('type')=='Ready' and c.get('status')=='True' for c in pod.get('status',{}).get('conditions',[])) and pod.get('status',{}).get('phase')=='Running'
 def components(self,require_ready=False):
  roles={n['id']:n['role'] for n in self.network['spec']['nodes']};roles.update({a['id']:'ca' for a in self.network['spec']['configuration']['authorities']})
  result={}
  for pod in self.pods():
   name=pod['metadata'].get('labels',{}).get('release')
   if name not in roles or pod['metadata'].get('deletionTimestamp'):continue
   require(name not in result,'duplicate-managed-pod')
   result[name]={'kind':roles[name],'pod':pod['metadata']['name'],'podUID':pod['metadata']['uid'],'node':pod['spec'].get('nodeName',''),'ready':self.ready(pod),'claims':[v['persistentVolumeClaim']['claimName'] for v in pod['spec'].get('volumes',[]) if 'persistentVolumeClaim' in v]}
  if require_ready:require(set(result)==set(roles) and all(c['ready'] and c['node'] in self.workers for c in result.values()),'managed-workloads-not-ready')
  return result
 def volumes(self):
  result={}
  for pvc in json.loads(self.kub('-n',self.ns,'get','pvc','-o','json'))['items']:
   require(pvc['metadata']['namespace']==self.ns and pvc.get('status',{}).get('phase')=='Bound','original-pvc-not-bound')
   name=pvc['spec']['volumeName'];pv=json.loads(self.kub('get','pv',name,'-o','json'));claim=pv['spec']['claimRef']
   require(claim.get('namespace')==self.ns and claim.get('name')==pvc['metadata']['name'] and claim.get('uid')==pvc['metadata']['uid'],'pv-claim-binding')
   host=pv_host(pv['spec'].get('nodeAffinity',{}));require(host in self.workers,'pv-outside-worker-inventory')
   result[pvc['metadata']['name']]={'uid':pvc['metadata']['uid'],'phase':'Bound','pv':name,'pvUID':pv['metadata']['uid'],'host':host}
  require(result,'pvc-inventory-empty');return result
 def identities(self):
  result={}
  for secret in json.loads(self.kub('-n',self.ns,'get','secrets','-o','json'))['items']:
   if secret.get('immutable') is True and secret['metadata'].get('labels',{}).get('sathonate.io/owner')=='issuer':
    require(secret['metadata'].get('labels',{}).get('sathonate.io/namespace')==self.ns,'identity-owner')
    result[secret['metadata']['name']]=(secret['metadata']['uid'],hashlib.sha256(json.dumps(secret.get('data',{}),sort_keys=True).encode()).hexdigest())
  require(result,'identity-inventory-empty');return result
 def pod_spec(self):
  return {'automountServiceAccountToken':False,'restartPolicy':'Never','securityContext':{'runAsNonRoot':True,'runAsUser':10001,'runAsGroup':10001,'seccompProfile':{'type':'RuntimeDefault'}},'containers':[{'name':'probe','image':self.config['workerImage'],'imagePullPolicy':'Never','command':['python','-c','import time;time.sleep(1200)'],'securityContext':{'allowPrivilegeEscalation':False,'readOnlyRootFilesystem':True,'capabilities':{'drop':['ALL']}},'resources':{'requests':{'cpu':'25m','memory':'32Mi'},'limits':{'cpu':'100m','memory':'128Mi'}}}]}
 def canary(self,choices):
  preferred=choices[0]['worker'];name='s6-reschedule-'+secrets.token_hex(6);pod=self.pod_spec();pod.pop('restartPolicy')
  pod['affinity']={'nodeAffinity':{'requiredDuringSchedulingIgnoredDuringExecution':{'nodeSelectorTerms':[{'matchExpressions':[{'key':'kubernetes.io/hostname','operator':'In','values':self.workers}]}]},'preferredDuringSchedulingIgnoredDuringExecution':[{'weight':100,'preference':{'matchExpressions':[{'key':'kubernetes.io/hostname','operator':'In','values':[preferred]}]}}]}}
  pod['tolerations']=[{'key':'node.kubernetes.io/'+key,'operator':'Exists','effect':'NoExecute','tolerationSeconds':5} for key in ('not-ready','unreachable')]
  self.canary_ref=self.create('Deployment',name,{'replicas':1,'selector':{'matchLabels':{'s6-canary':name}},'template':{'metadata':{'labels':{'s6-canary':name}},'spec':pod}})
  def observed():
   pods=[p for p in self.pods() if p['metadata'].get('labels',{}).get('s6-canary')==name and self.ready(p) and not p['metadata'].get('deletionTimestamp')]
   return pods[0] if len(pods)==1 else None
  original=self.wait(observed,120,'stateless-canary-not-ready');self.original_canary=original
  choice=next((c for c in choices if c['worker']==original['spec']['nodeName']),None);require(choice is not None,'canary-not-on-quorum-safe-worker');return choice
 def docker_target(self):
  info=json.loads(self.run(['docker','inspect','--format','{"id":{{json .Id}},"name":{{json .Name}},"labels":{{json .Config.Labels}},"running":{{json .State.Running}},"paused":{{json .State.Paused}}}',self.target_id or self.worker],timeout=15))
  require(info['labels'].get('io.x-k8s.kind.cluster')==self.cluster and info['labels'].get('io.x-k8s.kind.role')=='worker' and info['name']=='/'+self.worker,'docker-worker-owner')
  if self.target_id:require(info['id']==self.target_id,'worker-container-replaced')
  return info
 def start_api(self):
  private=self.cache/(self.cluster+'-s6-grants-'+secrets.token_hex(4));private.mkdir(exist_ok=False);self.private=private
  if os.name=='nt':self.run(['icacls',self.private,'/inheritance:r','/grant:r',self.run(['whoami']).strip()+':(OI)(CI)F'])
  else:self.private.chmod(0o700)
  tenant='sprint1-'+self.cluster.removeprefix('smp-sprint1-');grants=[]
  for role,t,p,permissions in [('primary',tenant,'project-a',['read','write','deploy','retry']),('viewer',tenant,'project-a',['read']),('foreignTenant','foreign-'+tenant,'project-a',['read','deploy']),('foreignProject',tenant,'project-b',['read','deploy'])]:
   token=secrets.token_hex(32);self.tokens[role]=token;grants.append({'tokenSHA256':hashlib.sha256(token.encode()).hexdigest(),'developer':'qualification-developer','tenant':t,'project':p,'permissions':permissions,'expiresAt':(datetime.now(timezone.utc)+timedelta(hours=1)).isoformat()})
  path=self.private/'grants.json'
  with path.open('x',encoding='utf-8') as output:
   self.grant_created=True;json.dump(grants,output)
  binary=self.root/('bin/fabric-api-sprint6.exe' if os.name=='nt' else 'bin/fabric-api-sprint6')
  build=subprocess.run(['go','build','-o',str(binary),'./cmd/fabric-api'],cwd=self.root,env=self.env,capture_output=True,timeout=180)
  require(build.returncode==0,'final-source-api-build')
  self.api_binary_sha=hashlib.sha256(binary.read_bytes()).hexdigest()
  with socket.socket() as listener:listener.bind(('127.0.0.1',0));port=listener.getsockname()[1]
  self.log_path=self.output/'worker-api.log';self.log_handle=self.log_path.open('w',encoding='utf-8')
  environment=dict(self.env,DATABASE_URL=self.env['SATHONATE_TEST_DATABASE_URL'],SATHONATE_APP_ENV='local-development')
  password=unquote(urlsplit(environment['DATABASE_URL']).password or '')
  values=list(self.tokens.values())+[password,environment['DATABASE_URL']]
  self.private_values=values+[base64.b64encode(v.encode()).decode() for v in values if v]
  self.process=subprocess.Popen([str(binary),'-mode','local-development','-deployment-mode','secure-local','-local-token-grants',str(path),'-secure-local-config',str(self.runtime),'-addr','127.0.0.1:'+str(port)],cwd=self.root,env=environment,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True,encoding='utf-8',errors='replace')
  def marked():self.log_exposure=True
  def stream_log():
   try:redact_log(self.process.stdout,self.log_handle,self.private_values,marked)
   except Exception:self.log_stream_failed=True
  self.log_thread=threading.Thread(target=stream_log,daemon=True);self.log_thread.start()
  self.api=API(port,self.tokens,self.private_values)
  def available():
   require(self.process.poll() is None,'authenticated-api-exited')
   if not listener_owned(port,self.process.pid):return False
   try:return self.api.request('/auth/config')[0]==200
   except OSError:return False
  self.wait(available,60,'authenticated-api-startup')
 def prepare(self):
  self.guard();self.start_api();status,self.before=self.api.request('/deployments/'+self.operation)
  require(status==200 and self.before.get('verified') is True and self.before.get('mode')=='secure-local','current-verified-server-operation')
  self.network=self.before['network'];require(self.network['spec']['deployment']['targetRef']=='smp-dynamic-local-v1','dynamic-local-only')
  require(self.api.request('/networks/'+self.network_id)[0]==200,'server-owned-network-binding')
  self.baseline_components=self.components(True);self.baseline_volumes=self.volumes();self.baseline_identities=self.identities()
  self.node_uids={n:self.node(n)['metadata']['uid'] for n in self.workers};require(all(self.node_ready(n) for n in self.workers),'workers-not-ready')
  require(all(c['claims'] and all(self.baseline_volumes[v]['host']==c['node'] for v in c['claims']) for c in self.baseline_components.values()),'original-pvc-host-binding')
  choices=candidates(self.network,self.workers,self.baseline_components);self.choice=self.canary(choices);self.worker=self.choice['worker']
  self.target_id=self.docker_target()['id'];require(self.docker_target()['running'] and not self.docker_target()['paused'],'worker-not-running')
  self.observer_node=next(n for n in self.workers if n!=self.worker);self.setup_tls_observer()
  require(self.raft_probe(False)['majority'],'preflight-raft-tls')
  return {'worker':self.worker,'workerContainerID':self.target_id,'namespace':self.ns,'nativeExecutableSHA256':self.api_binary_sha,'workerImage':self.config['workerImage'],'volumeCount':len(self.baseline_volumes),'immutableIdentityCount':len(self.baseline_identities),'quorumBudgets':self.choice['channels'],'physicalHostResilienceQualified':False}
 def authorize(self):
  path='/deployments/'+self.operation
  for role in ('foreignTenant','foreignProject'):require(self.api.request(path+'/workloads',role)[0]==404,'cross-scope-monitoring')
  require(self.api.request(path+'/workloads','invalid')[0]==401,'invalid-token')
  require(self.api.request(path+'/workloads',None)[0]==401,'missing-token')
  require(self.api.request(path+'/workloads','viewer')[0]==200,'viewer-read')
  require(self.api.request(path+'/recovery/deploy','viewer','POST',{'node':'','approval':'0'*64},{'Idempotency-Key':'denied-'+secrets.token_hex(8)})[0]==403,'viewer-recovery-escalation')
  status,preview=self.api.request(path+'/recovery/preview?node=');require(status==200,'recovery-preview')
  digest=preview['plan']['digest'];wrong=('0' if digest[0]!='0' else '1')+digest[1:]
  require(self.api.request(path+'/recovery/deploy','primary','POST',{'node':'','approval':wrong},{'Idempotency-Key':'stale-'+secrets.token_hex(8)})[0]==409,'stale-recovery-approval')
  return {'authorization':True}
 def pause(self,worker):
  self.guard();require(worker==self.worker,'pause-target');self.docker_target();self.paused_at=time.monotonic()
  self.watchdog=threading.Timer(180,self.expire_pause);self.watchdog.daemon=True;self.watchdog.start()
  self.run(['docker','pause',self.target_id],timeout=15)
  require(self.docker_target()['paused'] is True,'pause-unacknowledged')
 def expire_pause(self):
  self.watchdog_fired=True
  try:self.resume(self.worker)
  except Exception:self.watchdog_error=True
 def resume(self,worker):
  with self.resume_lock:
   require(worker==self.worker,'resume-target');info=self.docker_target()
   if info['paused']:self.run(['docker','unpause',self.target_id],timeout=15)
   info=self.docker_target();require(info['running'] and not info['paused'],'resume-unacknowledged')
   if self.watchdog:self.watchdog.cancel()
 def setup_tls_observer(self):
  self.orgs=sorted({n['organizationRef'] for n in self.network['spec']['nodes'] if n['role']=='orderer'})
  pod=self.pod_spec();pod['nodeSelector']={'kubernetes.io/hostname':self.observer_node};pod['volumes']=[];mounts=[]
  for i,org in enumerate(self.orgs):
   name=org+'-channel-tls-identity';secret=json.loads(self.kub('-n',self.ns,'get','secret',name,'-o','json'));labels=secret['metadata'].get('labels',{})
   require(secret.get('immutable') is True and labels.get('sathonate.io/owner')=='issuer' and labels.get('sathonate.io/namespace')==self.ns,'participation-identity-owner')
   pod['volumes'].append({'name':'tls-'+str(i),'secret':{'secretName':name}});mounts.append({'name':'tls-'+str(i),'mountPath':'/material/'+str(i),'readOnly':True})
  pod['containers'][0]['volumeMounts']=mounts
  self.observer=self.create('Pod','s6-tls-'+secrets.token_hex(6),pod);self.kub('-n',self.ns,'wait','pod/'+self.observer['name'],'--for=condition=Ready','--timeout=90s')
 def raft_probe(self,outage):
  nodes={n['id']:n for n in self.network['spec']['nodes']};requests=[]
  for budget in self.choice['channels']:
   for name in budget['remaining']:
    requests.append({'channel':budget['channel'],'name':name,'orgIndex':self.orgs.index(nodes[name]['organizationRef']),'url':'https://'+name+'.'+self.ns+':7053/participation/v1/channels/'+budget['channel']})
  script="import sys,json,ssl,urllib.request;items=json.load(sys.stdin);out=[]\nfor item in items:\n p='/material/'+str(item['orgIndex']);c=ssl.create_default_context(cafile=p+'/root.pem');c.minimum_version=ssl.TLSVersion.TLSv1_2;c.load_cert_chain(p+'/cert.pem',p+'/key.pem')\n with urllib.request.urlopen(item['url'],context=c,timeout=5) as r:v=json.load(r)\n out.append({'name':item['name'],'channel':item['channel'],'status':v.get('status'),'relation':v.get('consensusRelation'),'height':v.get('height')})\nprint(json.dumps(out))"
  rows=json.loads(self.kub('-n',self.ns,'exec','-i',self.observer['name'],'--','python','-c',script,data=json.dumps(requests)))
  require({(r.get('name'),r.get('channel')) for r in rows}=={(r['name'],r['channel']) for r in requests},'raft-membership-response-mismatch')
  heights={c['id']:max(v['health']['peers'][0]['height'] for v in self.before['verifications'] if v['health']['channel']==c['id']) for c in self.network['spec']['channels']}
  require(len(rows)==len(requests) and all(r.get('status')=='active' and r.get('relation')=='consenter' and isinstance(r.get('height'),int) and r['height']>=heights[r['channel']] for r in rows),'healthy-raft-participation')
  for budget in self.choice['channels']:require(len([r for r in rows if r['channel']==budget['channel']])>=budget['required'],'raft-majority-not-observed')
  cross=any(self.baseline_components[r['name']]['node']!=self.observer_node for r in rows)
  require(cross,'cross-worker-tls-not-exercised')
  require(not self.watchdog_fired,'pause-watchdog-fired')
  if outage:self.quorum_receipt=rows
  return {'majority':True,'crossWorkerTLS':cross,'rows':rows}
 def observe_outage(self,worker):
  self.wait(lambda:not self.node_ready(worker),120,'node-not-ready-not-observed',True)
  def degraded():
   status,value=self.api.request('/deployments/'+self.operation+'/workloads')
   if status==200 and value.get('workloadsReady') is False:
    self.degraded_monitoring=value;return True
   return False
  self.wait(degraded,30,'monitoring-did-not-degrade',True)
  status,history=self.api.request('/operations/'+self.operation)
  require(status==200 and history.get('status')=='succeeded' and any(e.get('message','').startswith('Monitoring checkpoint: ready=false;') for e in history.get('events',[])),'degraded-history-not-durable')
  def rescheduled():
   pods=[p for p in self.pods() if p['metadata'].get('labels',{}).get('s6-canary')==self.canary_ref['name'] and not p['metadata'].get('deletionTimestamp') and self.ready(p) and p['metadata']['uid']!=self.original_canary['metadata']['uid'] and p['spec'].get('nodeName') in self.workers and p['spec']['nodeName']!=worker]
   return pods[0] if len(pods)==1 else None
  new=self.wait(rescheduled,120,'stateless-rescheduling-not-observed',True);self.reschedule_seconds=time.monotonic()-self.paused_at
  volume=next((v for c in self.baseline_components.values() if c['node']==worker for v in c['claims']),None);require(volume is not None,'worker-has-no-original-local-pvc')
  pod=self.pod_spec();pod['affinity']={'nodeAffinity':{'requiredDuringSchedulingIgnoredDuringExecution':{'nodeSelectorTerms':[{'matchExpressions':[{'key':'kubernetes.io/hostname','operator':'In','values':[n for n in self.workers if n!=worker]}]}]}}}
  pod['volumes']=[{'name':'original','persistentVolumeClaim':{'claimName':volume,'readOnly':True}}];pod['containers'][0]['volumeMounts']=[{'name':'original','mountPath':'/original','readOnly':True}]
  probe=self.create('Pod','s6-pv-affinity-'+secrets.token_hex(6),pod)
  def blocked():
   value=json.loads(self.kub('-n',self.ns,'get','pod',probe['name'],'-o','json'));require(not value['spec'].get('nodeName'),'local-pv-unexpectedly-migrated')
   return any(c.get('type')=='PodScheduled' and c.get('status')=='False' and c.get('reason')=='Unschedulable' and any(marker in c.get('message','').lower() for marker in ('volume node affinity conflict',"didn't match persistentvolume's node affinity")) for c in value.get('status',{}).get('conditions',[]))
  self.wait(blocked,30,'local-pv-affinity-not-observed',True);unchanged_storage(self.baseline_volumes,self.volumes())
  current=self.components();require(set(self.choice['lost']).issubset(current),'stateful-observation-incomplete');require(all(c['node']==worker for name,c in current.items() if name in self.choice['lost']),'stateful-workload-migrated')
  raft=self.raft_probe(True);require(time.monotonic()-self.paused_at<180,'bounded-pause-deadline')
  self.outage_details={'statelessNewPodUID':new['metadata']['uid'],'statelessNewNode':new['spec']['nodeName'],'statelessReschedulingSeconds':self.reschedule_seconds,'readOnlyPVCProbe':volume,'healthyRaftParticipation':raft['rows'],'monitoring':{'workloadsReady':self.degraded_monitoring['workloadsReady'],'missing':self.degraded_monitoring.get('missing',[]),'alerts':self.degraded_monitoring.get('alerts',[])},'terminalOperationStatusPreserved':'succeeded'}
  return dict.fromkeys(['nodeNotReadyObserved','monitoringDegraded','statelessRescheduled','localPVProbeUnschedulable','originalPVCsBoundDuringOutage','raftHealthyMajorityViaMutualTLS','crossWorkerTLS'],True)
 def recover(self,worker):
  require(not self.watchdog_fired,'pause-watchdog-fired')
  start=time.monotonic()
  def ready():return self.node_ready(worker) and len(self.components())==len(self.baseline_components) and all(c['ready'] for c in self.components().values())
  self.wait(ready,240,'original-worker-recovery-timeout');node_return=time.monotonic()-start
  require(all(self.node(n)['metadata']['uid']==uid for n,uid in self.node_uids.items()),'node-object-replaced');self.docker_target()
  unchanged_storage(self.baseline_volumes,self.volumes());require(self.identities()==self.baseline_identities,'immutable-identity-changed')
  components=self.components(True);require(all(c['node']==self.baseline_components[n]['node'] for n,c in components.items()),'local-stateful-workload-moved')
  preserved=[]
  for verification in self.before['verifications']:
   channel=verification['health']['channel'];tx=verification['transactionId']
   for peer in verification['health']['peers']:
    component=components[peer['peer']];request=json.dumps({'Args':['GetTransactionByID',channel,tx]},separators=(',',':'))
    raw=self.kub('-n',self.ns,'exec',component['pod'],'--','env','CORE_PEER_ADDRESS='+peer['peer']+'.'+self.ns+':7051','CORE_PEER_TLS_ENABLED=true','CORE_PEER_TLS_ROOTCERT_FILE=/var/hyperledger/tls/server/cert/cacert.pem','peer','chaincode','query','-C',channel,'-n','qscc','--hex','-c',request).strip()
    valid_transaction(bytes.fromhex(raw),tx);preserved.append({'channel':channel,'peer':peer['peer'],'transactionId':tx,'validationCode':0})
  path='/deployments/'+self.operation;status,preview=self.api.request(path+'/recovery/preview?node=');require(status==200,'post-worker-recovery-preview')
  payload={'node':'','approval':preview['plan']['digest']};headers={'Idempotency-Key':'s6-worker-'+secrets.token_hex(12)}
  status,op=self.api.request(path+'/recovery/deploy','primary','POST',payload,headers);require(status==202,'post-worker-api-recovery-start')
  status,replayed=self.api.request(path+'/recovery/deploy','primary','POST',payload,headers);require(status==202 and replayed.get('operationId')==op.get('operationId'),'recovery-not-idempotent')
  def finished():
   status,value=self.api.request('/operations/'+op['operationId']);require(status==200,'recovery-operation-hidden')
   require(value.get('status') not in ('failed','cancelled','interrupted'),'fresh-fabric-verification-failed')
   return value if value.get('status')=='succeeded' else None
  self.wait(finished,720,'fresh-fabric-verification-timeout');status,after=self.api.request('/deployments/'+op['operationId']);require(status==200 and after.get('verified') is True,'fresh-verification-result')
  require(len(after['verifications'])==len(self.before['verifications']),'channel-verification-incomplete')
  for value in after['verifications']:
   old=next(v for v in self.before['verifications'] if v['health']['channel']==value['health']['channel'] and v['health']['chaincode']['name']==value['health']['chaincode']['name'])
   require(value['transactionId']!=old['transactionId'] and value['health']['ready'] is True,'fresh-transaction-missing')
   peers=value['health']['peers'];previous={p['peer']:p for p in old['health']['peers']}
   require(set(previous)=={p['peer'] for p in peers},'peer-membership-changed')
   require(all(p['available'] is True and p['height']>previous[p['peer']]['height'] and p['height']==peers[0]['height'] and p['currentBlockHash']==peers[0]['currentBlockHash'] for p in peers),'post-recovery-ledgers-diverged')
  status,health=self.api.request('/deployments/'+op['operationId']+'/workloads');require(status==200 and health.get('workloadsReady') is True,'monitoring-not-ready')
  unchanged_storage(self.baseline_volumes,self.volumes());require(self.identities()==self.baseline_identities,'post-reconcile-identity-changed')
  final=self.components(True);require(all(final[n]['podUID']==c['podUID'] for n,c in components.items()),'reconcile-redundant-restart')
  checks=dict.fromkeys(['sameWorkerRecovered','PVCAndPVUIDsPreserved','immutableIdentitiesPreserved','priorTransactionsVALID','freshAPITransactionsCommitted','allPeerLedgersConverged','recoveryIdempotent','monitoringReady'],True)
  return {'checks':checks,'evidence':{'operationId':op['operationId'],'priorTransactions':preserved,'verifications':after['verifications'],'outage':self.outage_details,'nodeReadySeconds':node_return,'noRedundantReconcileRestart':True}}
 def cleanup(self):
  errors=[];resource_clean=True;unpaused=True;private_clean=True;api_stopped=True;log_clean=not self.log_exposure
  if self.watchdog:self.watchdog.cancel()
  if self.target_id:
   try:self.resume(self.worker)
   except Exception:unpaused=False;errors.append('worker-resume')
  for ref in reversed(self.created):
   try:self.delete(ref)
   except Exception:resource_clean=False;errors.append('uid-resource-cleanup')
  try:
   if self.process:
    if self.process.poll() is None:self.process.terminate()
    try:self.process.wait(timeout=15)
    except subprocess.TimeoutExpired:self.process.kill();self.process.wait(timeout=10)
  except Exception:errors.append('api-stop')
  finally:
   if self.process:
    try:api_stopped=self.process.poll() is not None
    except Exception:api_stopped=False;errors.append('api-state-unavailable')
   try:
    if self.log_thread:
     self.log_thread.join(timeout=10)
     if self.log_thread.is_alive():log_clean=False;errors.append('api-log-stream-cleanup')
    if self.process and self.process.stdout:self.process.stdout.close()
    if self.log_handle:self.log_handle.close()
    if hasattr(self,'log_path'):
     text=self.log_path.read_text(encoding='utf-8',errors='replace');log_clean=log_clean and not any(value in text for value in self.private_values if value) and 'PRIVATE KEY' not in text
   except Exception:log_clean=False;errors.append('api-log-cleanup')
   finally:
    if self.private:
     try:
      if self.grant_created:(self.private/'grants.json').unlink(missing_ok=True)
      self.private.rmdir()
     except OSError:private_clean=False;errors.append('private-grants-cleanup')
  return {'workerUnpaused':unpaused,'uidCheckedResourcesRemoved':resource_clean,'privateGrantsRemoved':private_clean,'apiStopped':api_stopped,'nativeAPIPID':getattr(self.process,'pid',None) if self.process else None,'credentialInspectionClean':log_clean and not self.log_stream_failed,'pauseBounded':not self.watchdog_fired and not self.watchdog_error,'errorCategories':errors}

def qualify(run,kub,guard,root,cache,runtime,env,initial,output,report):
 require(report.get('capacity',{}).get('status')=='PASS_PREFLIGHT_ONLY','capacity-not-qualified')
 adapter=Live(run,kub,guard,root,cache,runtime,env,initial,output)
 path=output/'worker-recovery.json'
 def save(value):
  temp=path.with_suffix('.pending');temp.write_text(json.dumps(value,indent=2)+'\n',encoding='utf-8');os.replace(temp,path)
 return run_case(adapter,save)
