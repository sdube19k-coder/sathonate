"""Disposable-only acceptance for offline cold volumes and retained-data deletion.
Never usable against the retained cluster; provider ID and namespace bindings are
checked before each destructive fixture operation. No private payload is logged.
"""
import base64,gzip,hashlib,json,os,re,secrets,subprocess,time,urllib.request
from urllib.parse import urlsplit,unquote
from datetime import datetime,timezone

def qualify(run,kub,apply,guard,report,result,backend,cache,runtime_path,env,tokens,tenant,api_process,restart_api,log):
 guard();namespace=result['namespace'];operation=result['final']['operationId'];network=result['networkId']
 if os.environ.get('SMP_UI_SPRINT5_DELETION_ONLY')=='1':
  volumes={x['metadata']['name']:x['metadata']['uid'] for x in json.loads(kub('-n',namespace,'get','pvc','-o','json'))['items']}
  qualify_deletion(run,kub,apply,guard,report,result,backend,runtime_path,env,tokens,tenant,log,result['final'],volumes)
  return
 scope='lp-'+hashlib.sha256(json.dumps([tenant,'project-a'],separators=(',',':')).encode()).hexdigest()[:48]
 if os.environ.get('SMP_UI_SPRINT7')=='1':
  expected_namespace='smp-s-'+hashlib.sha256(json.dumps([scope,'token-bound-project',network],separators=(',',':')).encode()).hexdigest()[:32]
  if namespace!=expected_namespace or tenant!=report['cluster']:raise RuntimeError('Browser result does not bind the disposable recovery target')
 private=cache/(report['cluster']+'-cold-private');private.mkdir(exist_ok=False)
 try:
  if os.name=='nt':run(['icacls',private,'/inheritance:r','/grant:r',run(['whoami']).strip()+':(OI)(CI)F'])
  else:private.chmod(0o700)
  key=private/'key.bin';key.write_bytes(secrets.token_bytes(32))
  wrong=private/'wrong-key.bin';wrong.write_bytes(secrets.token_bytes(32))
  artifact=private/'cold.encrypted.json'
  binding_file=private/'binding.json'
  scheduled=os.environ.get('SMP_UI_SPRINT7')=='1'
  tool=private/('cold-tool.exe' if os.name=='nt' else 'cold-tool') if scheduled else backend/'bin/fabric-cold-backup.exe'
  build_env=dict(env,GOCACHE=str(cache/'go-build'),GOPATH=str(cache/'go'),GOPROXY='off',GOOS='windows' if os.name=='nt' else 'linux',GOARCH='amd64')
  if scheduled:
   build=subprocess.run(['go','build','-o',str(tool),'./cmd/fabric-cold-backup'],cwd=backend,env=build_env,capture_output=True,timeout=180)
   if build.returncode:raise RuntimeError('Cold helper final-source build failed')
   report['coldToolBinarySHA256']=hashlib.sha256(tool.read_bytes()).hexdigest()
  receipts=[]
  minimal=os.environ.get('SMP_UI_SPRINT5_COLD_MINIMAL')=='1'

  def cold(action,key_path=key,file=None,expected=True):
   guard()
   file=artifact if file is None else file
   if not scheduled and action in ('restore','resume'):
    build_env=dict(env,GOCACHE=str(cache/'go-build'),GOPATH=str(cache/'go'),GOPROXY='off',GOOS='windows' if os.name=='nt' else 'linux',GOARCH='amd64')
    build=subprocess.run(['go','build','-o',str(tool),'./cmd/fabric-cold-backup'],cwd=backend,env=build_env,capture_output=True,timeout=180)
    if build.returncode:raise RuntimeError('Cold helper final-source build failed')
    report['coldToolBinarySHA256']=hashlib.sha256(tool.read_bytes()).hexdigest()
   args=[str(tool),'-secure-local-config',str(runtime_path),'-action',action,'-tenant',scope,'-operation',operation,'-key-file',str(key_path)]
   if action=='scheduled-capture':args+=['-schedule-directory',str(private),'-schedule-hour-utc',str(schedule_hour),'-schedule-keep','2']
   else:args+=['-artifact',str(file)]
   if action=='restore' and binding_file.exists():args+=['-binding',str(binding_file)]
   p=subprocess.run(args,env=dict(env,SATHONATE_APP_ENV='local-development'),capture_output=True,text=True,timeout=1560)
   if expected:
    if p.returncode:raise RuntimeError('Cold '+action+' acceptance failed: '+(p.stderr.strip()[:512] if p.stderr.startswith('Cold maintenance refused:') else 'private diagnostics withheld'))
    value=json.loads(p.stdout);receipts.append(value);return value
   if p.returncode==0:raise RuntimeError('Unsafe cold '+action+' unexpectedly succeeded')
  api=local_api(tokens)
  # Inspection probes have completed; remove only the exact tokenless fixture pods.
  for name in ('secure-tls-observer','isolation-probe'):
   guard();kub('-n',namespace,'delete','pod',name,'--ignore-not-found','--wait=true','--timeout=60s')
  if not minimal:
   cold('capture',expected=False)
   if json.loads(kub('-n',namespace,'get','namespace',namespace,'-o','json'))['metadata'].get('labels',{}).get('fabric.sathonate.io/cold-maintenance'):raise RuntimeError('Active executor rejection mutated the namespace')
  api_process.terminate();api_process.wait(timeout=20)
  # The offline boundary permits replacing the stopped Windows executable.
  # Build both tools from the final workspace before restore/deletion acceptance.
  build_env=dict(env,GOCACHE=str(cache/'go-build'),GOPATH=str(cache/'go'),GOPROXY='off',GOOS='windows' if os.name=='nt' else 'linux',GOARCH='amd64')
  for executable,package in ([] if scheduled else [('fabric-api-sprint5.exe','./cmd/fabric-api'),('fabric-cold-backup.exe','./cmd/fabric-cold-backup')]):
   p=subprocess.run(['go','build','-o',str(backend/'bin'/executable),package],cwd=backend,env=build_env,capture_output=True,timeout=180)
   if p.returncode:raise RuntimeError('Final-source offline executable build failed; diagnostics withheld')
  report['postColdAPIBinarySHA256']=report['provenance']['apiBinarySHA256'] if scheduled else hashlib.sha256((backend/'bin/fabric-api-sprint5.exe').read_bytes()).hexdigest()
  report['coldToolBinarySHA256']=hashlib.sha256(tool.read_bytes()).hexdigest()
  report['finalMaintenanceSourceFiles']={path.relative_to(backend).as_posix():hashlib.sha256(path.read_bytes().replace(b'\r\n',b'\n')).hexdigest() for path in list((backend/'internal').rglob('*.go'))+list((backend/'cmd').rglob('*.go')) if not path.name.endswith('_test.go')}
  if scheduled:
   schedule_hour=datetime.now(timezone.utc).hour
   captured=cold('scheduled-capture')
   files=list(private.glob('????-??-??-*.encrypted.json'))
   if len(files)!=1 or captured.get('action')!='capture-resume' or captured.get('workloadsQuiesced') or hashlib.sha256(files[0].read_bytes()).hexdigest()!=captured.get('artifactSHA256'):raise RuntimeError('Scheduled capture/resume receipt or artifact mismatch')
   artifact=files[0]
   if datetime.now(timezone.utc).hour!=schedule_hour:raise RuntimeError('UTC schedule window changed before duplicate acknowledgement')
   if cold('scheduled-capture')!={'status':'skipped'}:raise RuntimeError('Scheduled slot was not idempotent')
   cold('prepare-restore',key_path=wrong,expected=False)
   if json.loads(kub('get','namespace',namespace,'-o','json'))['metadata'].get('labels',{}).get('fabric.sathonate.io/cold-maintenance'):raise RuntimeError('Invalid scheduled artifact created a fence')
   prepared=cold('prepare-restore')
   if not prepared.get('workloadsQuiesced') or prepared.get('artifactSHA256')!=captured['artifactSHA256']:raise RuntimeError('Scheduled restore preparation did not bind the exact captured artifact')
   report['checks']['scheduledCaptureAndExactRestorePreparation']='PASS'
  else:
   captured=cold('capture')
  (log/'cold-progress.json').write_text(json.dumps({'status':'IN_PROGRESS','capture':captured,'stage':'CAPTURED'},indent=2))
  binding_file.write_text(json.dumps(captured['binding']))
  plan=json.loads(json.loads(kub('-n',namespace,'get','configmap','sathonate-cold-checkpoint','-o','json'))['data']['plan.json'])
  if plan['Namespace']!=namespace or plan['Tenant']!=scope or plan['Operation']!=operation:raise RuntimeError('Cold checkpoint scope mismatch')
  if 'PRIVATE KEY' in artifact.read_text():raise RuntimeError('Plaintext in encrypted artifact')
  tampered=private/'tampered.encrypted.json'
  if not minimal:
   cold('restore',key_path=wrong,expected=False)
   tampered=private/'tampered.encrypted.json';raw=artifact.read_bytes();tampered.write_bytes(raw[:-8]+bytes([raw[-8]^1])+raw[-7:]);cold('restore',file=tampered,expected=False)
   # A restore must not overwrite the still-present live data.
   cold('restore',expected=False)
  volume_uids={v['Name']:v['UID'] for v in plan['Volumes']}
  worker=json.loads(runtime_path.read_text())['workerImage']
  wiped=[]
  # Destructive data-loss injection is fixture-only, after a successful capture.
  for v in plan['Volumes']:
   guard();claim=json.loads(kub('-n',namespace,'get','pvc',v['Name'],'-o','json'))
   if claim['metadata']['uid']!=v['UID'] or claim['status']['phase']!='Bound':raise RuntimeError('Fixture original claim mismatch')
   name='cold-loss-fixture-'+hashlib.sha256(v['Name'].encode()).hexdigest()[:12]
   apply({'apiVersion':'v1','kind':'Pod','metadata':{'name':name,'namespace':namespace},'spec':{'automountServiceAccountToken':False,'restartPolicy':'Never','terminationGracePeriodSeconds':1,'securityContext':{'runAsUser':0,'runAsGroup':0,'seccompProfile':{'type':'RuntimeDefault'}},'containers':[{'name':'fault','image':worker,'imagePullPolicy':'Never','command':['sleep','300'],'securityContext':{'allowPrivilegeEscalation':False,'capabilities':{'drop':['ALL'],'add':['DAC_OVERRIDE','CHOWN','FOWNER']}},'volumeMounts':[{'name':'data','mountPath':'/volume'}]}],'volumes':[{'name':'data','persistentVolumeClaim':{'claimName':v['Name']}}]}})
   try:
    kub('-n',namespace,'wait','pod/'+name,'--for=condition=Ready','--timeout=90s')
    script="import pathlib,os,shutil;p=pathlib.Path('/volume');assert p.resolve()==p and os.path.ismount(p);[(shutil.rmtree(x) if x.is_dir() and not x.is_symlink() else x.unlink()) for x in p.iterdir() if x.name!='lost+found'];print('EMPTY')"
    if kub('-n',namespace,'exec',name,'--','python','-c',script).strip()!='EMPTY':raise RuntimeError('Disposable empty-volume injection failed')
   finally:kub('-n',namespace,'delete','pod',name,'--wait=true','--timeout=60s')
   wiped.append(v['Name'])
  # Delete only two freshly enrolled, captured peer identities and this one
  # disposable network's control rows. Existing memberships/other tenants remain.
  identity_loss=[]
  for peer in result['final']['verifications'][0]['health']['peers']:
   guard();name=peer['peer']+'-tls-identity';secret=json.loads(kub('-n',namespace,'get','secret',name,'-o','json'))
   if secret['metadata']['namespace']!=namespace:raise RuntimeError('Identity loss scope mismatch')
   digest=hashlib.sha256(json.dumps(secret.get('data',{}),sort_keys=True).encode()).hexdigest()
   kub('-n',namespace,'delete','--raw','/api/v1/namespaces/'+namespace+'/secrets/'+name,'-f','-',data=json.dumps({'apiVersion':'v1','kind':'DeleteOptions','preconditions':{'uid':secret['metadata']['uid']}}))
   identity_loss.append({'name':name,'originalUID':secret['metadata']['uid'],'dataSHA256':digest})
  guard()
  postgres=json.loads(run(['docker','inspect','smp-postgres']))[0]
  pgvars=dict(x.split('=',1) for x in postgres['Config']['Env'] if '=' in x)
  literal=lambda value:"'"+value.replace("'","''")+"'"
  tenant_sql=literal(scope);network_sql=literal(network)
  script="BEGIN; SELECT set_config('sathonate.tenant_id',"+tenant_sql+",true); DELETE FROM sathonate_deployments WHERE tenant_id="+tenant_sql+" AND operation_id IN(SELECT id FROM sathonate_operations WHERE tenant_id="+tenant_sql+" AND network_id="+network_sql+"); DELETE FROM sathonate_operations WHERE tenant_id="+tenant_sql+" AND network_id="+network_sql+"; DELETE FROM sathonate_network_revisions WHERE tenant_id="+tenant_sql+" AND network_id="+network_sql+"; DELETE FROM sathonate_networks WHERE tenant_id="+tenant_sql+" AND id="+network_sql+"; COMMIT;"
  run(['docker','exec','-i','smp-postgres','psql','-X','-q','-v','ON_ERROR_STOP=1','-U',pgvars['POSTGRES_USER'],'-d','fabric_smp_tests'],data=script)
  restored=cold('restore')
  (log/'cold-progress.json').write_text(json.dumps({'status':'IN_PROGRESS','capture':captured,'restore':restored,'stage':'RESTORED'},indent=2))
  resumed=cold('resume')
  for identity in identity_loss:
   secret=json.loads(kub('-n',namespace,'get','secret',identity['name'],'-o','json'))
   if secret['metadata']['uid']==identity['originalUID'] or hashlib.sha256(json.dumps(secret.get('data',{}),sort_keys=True).encode()).hexdigest()!=identity['dataSHA256']:raise RuntimeError('Exact captured identity recovery failed')

  if restored['volumes']!=len(volume_uids) or resumed['workloadsQuiesced']:raise RuntimeError('Cold restoration receipt incomplete')
  if {x['metadata']['name']:x['metadata']['uid'] for x in json.loads(kub('-n',namespace,'get','pvc','-o','json'))['items']}!=volume_uids:raise RuntimeError('Cold recovery replaced a PVC')
  # Independently query the previously committed transaction from every peer
  # using its existing enrolled identity and verified peer TLS endpoint.
  preserved=[]
  pods=json.loads(kub('-n',namespace,'get','pods','-o','json'))['items']
  def validation_code(raw):
   pos=0;code=0
   def varint():
    nonlocal pos
    value=0;shift=0
    while pos<len(raw) and shift<70:
     b=raw[pos];pos+=1;value|=(b&127)<<shift
     if not b&128:return value
     shift+=7
    raise RuntimeError('Malformed transaction verification response')
   while pos<len(raw):
    tag=varint();field=tag>>3;wire=tag&7
    if wire==0:
     value=varint()
     if field==2:code=value
    elif wire==2:
     length=varint();pos+=length
    elif wire==1:pos+=8
    elif wire==5:pos+=4
    else:raise RuntimeError('Unexpected transaction response framing')
   return code
  for verification in result['final']['verifications']:
   channel=verification['health']['channel'];txid=verification['transactionId']
   for peer in verification['health']['peers']:
    pod=next(x['metadata']['name'] for x in pods if x['metadata'].get('labels',{}).get('release')==peer['peer'])
    request=json.dumps({'Args':['GetTransactionByID',channel,txid]},separators=(',',':'))
    encoded=kub('-n',namespace,'exec',pod,'--','env','CORE_PEER_ADDRESS='+peer['peer']+'.'+namespace+':7051','CORE_PEER_TLS_ROOTCERT_FILE=/var/hyperledger/tls/server/cert/cacert.pem','peer','chaincode','query','-C',channel,'-n','qscc','--hex','-c',request).strip()
    response=bytes.fromhex(encoded)
    if txid.encode() not in response or validation_code(response)!=0:raise RuntimeError('Previously committed transaction absent or invalid after cold restore')
    preserved.append({'peer':peer['peer'],'channel':channel,'transactionId':txid,'validationCode':0})
  (log/'cold-progress.json').write_text(json.dumps({'status':'IN_PROGRESS','capture':captured,'restore':restored,'resume':resumed,'priorTransactionsPreserved':preserved,'stage':'LEDGER_VERIFIED_BEFORE_API_RESTART'},indent=2))
  restart_api()
  for _ in range(90):
   try:
    status,config=api('/auth/config')
    if status==200:break
   except OSError:pass
   time.sleep(1)
  else:raise RuntimeError('API did not return after cold resume')
  status,preview=api('/deployments/'+operation+'/recovery/preview?node=')
  if status!=200:raise RuntimeError('Post-restore reconciliation preflight failed')
  status,op=api('/deployments/'+operation+'/recovery/deploy',method='POST',body={'node':'','approval':preview['plan']['digest']},headers={'Idempotency-Key':'cold-verify-'+secrets.token_hex(12)})
  if status!=202:raise RuntimeError('Post-restore API verification not started')
  deadline=time.monotonic()+720
  while time.monotonic()<deadline:
   status,current_op=api('/operations/'+op['operationId'])
   if status!=200 or current_op['status'] in ('failed','cancelled','interrupted'):raise RuntimeError('Post-restore Fabric verification failed')
   if current_op['status']=='succeeded':break
   time.sleep(1)
  else:raise RuntimeError('Post-restore Fabric verification timeout')
  status,current=api('/deployments/'+op['operationId'])
  if status!=200 or not current['verified'] or current['transactionId']==result['final']['transactionId']:raise RuntimeError('Post-restore fresh transaction missing')
  expected_channels={v['health']['channel'] for v in result['final']['verifications']}
  if not expected_channels or {v['health']['channel'] for v in current['verifications']}!=expected_channels or len(current['verifications'])!=len(result['final']['verifications']):raise RuntimeError('Post-restore channel coverage differs')
  for verification in current['verifications']:
   peers=verification['health']['peers'];prior=next(x for x in result['final']['verifications'] if x['health']['channel']==verification['health']['channel'])
   if not peers or {x['peer'] for x in peers}!={x['peer'] for x in prior['health']['peers']} or not verification.get('transactionId') or verification['transactionId']==prior['transactionId'] or not verification['health']['ready'] or not all(x['height']==peers[0]['height'] and x['currentBlockHash']==peers[0]['currentBlockHash'] and x['height']>prior['health']['peers'][0]['height'] for x in peers):raise RuntimeError('Post-restore ledger continuity unverified')
  evidence={'status':'PASS_LOCAL_COLD_VOLUME_RECOVERY','capture':captured,'restore':restored,'resume':resumed,'volumesRestored':wiped,'pvcUIDsPreserved':True,'priorTransactionsPreserved':preserved,'identitySecretsRecovered':identity_loss,'scopedControlRowsRecovered':True,'negativeChecks':'REUSED_FROM_20261107_130435' if minimal else 'PASS_EXECUTED_IN_THIS_RUN','activeExecutorRejected':None if minimal else True,'wrongKeyRejected':None if minimal else True,'tamperRejected':None if minimal else True,'nonemptyVolumeRejected':None if minimal else True,'postRestore':current,'limits':['Only missing scoped network control rows and missing captured Secrets are recreated; existing divergent data, authentication memberships, database roles and schema are never overwritten.','Same-namespace, original-PVC cold recovery only; physical multi-server/disaster recovery remains unqualified.']}
  if scheduled:
   evidence['scheduledArtifactSHA256']=captured['artifactSHA256']
   evidence['prepared']=prepared
  (log/'cold-volume-recovery.json').write_text(json.dumps(evidence,indent=2)+'\n')
  report['checks']['offlineColdVolumeRecovery']='PASS';report['coldVolumeRecovery']=evidence
  qualify_deletion(run,kub,apply,guard,report,result,backend,runtime_path,env,tokens,tenant,log,current,volume_uids)
 finally:
  clean=True
  scheduled_names=[p.name for p in private.iterdir() if re.fullmatch(r'\d{4}-\d{2}-\d{2}-[a-f0-9]{64}\.(started|complete|encrypted\.json)',p.name)]
  for name in ('key.bin','wrong-key.bin','cold.encrypted.json','tampered.encrypted.json','binding.json','cold-tool.exe','cold-tool',*scheduled_names):
   try:(private/name).unlink(missing_ok=True)
   except OSError:clean=False
  try:private.rmdir()
  except OSError:clean=False
  report['checks']['coldPrivateArtifactsRemoved']='PASS' if clean else 'FAIL'


def qualify_deletion(run,kub,apply,guard,report,result,backend,runtime_path,env,tokens,tenant,log,current,volume_uids):
 namespace=result['namespace'];network=result['networkId'];worker=json.loads(runtime_path.read_text())['workerImage']
 api=local_api(tokens)
 identity_digests={x['metadata']['name']:(x['metadata']['uid'],hashlib.sha256(json.dumps(x.get('data',{}),sort_keys=True).encode()).hexdigest()) for x in json.loads(kub('-n',namespace,'get','secrets','-o','json'))['items']}
 for name in ('secure-tls-observer','isolation-probe'):
  guard();kub('-n',namespace,'delete','pod',name,'--ignore-not-found','--wait=true','--timeout=60s')
 # A foreign workload must block the destructive plan, and a foreign
 # ConfigMap must survive this namespace's explicitly scoped workload removal.
 guard()
 apply({'apiVersion':'v1','kind':'ConfigMap','metadata':{'name':'foreign-preservation-canary','namespace':namespace},'data':{'canary':'disposable-owned-scope-test'}})
 canary=json.loads(kub('-n',namespace,'get','configmap','foreign-preservation-canary','-o','json'))['metadata']['uid']
 apply({'apiVersion':'apps/v1','kind':'Deployment','metadata':{'name':'foreign-deletion-canary','namespace':namespace},'spec':{'replicas':0,'selector':{'matchLabels':{'app':'foreign-deletion-canary'}},'template':{'metadata':{'labels':{'app':'foreign-deletion-canary'}},'spec':{'automountServiceAccountToken':False,'containers':[{'name':'canary','image':worker,'command':['sleep','60']}]}}}})
 status,_=api('/deployments/'+current['operationId']+'/cleanup-preview',token=tokens['administrator'])
 if status!=409:raise RuntimeError('Foreign workload did not block deletion preview')
 foreign=json.loads(kub('-n',namespace,'get','deployment','foreign-deletion-canary','-o','json'))
 kub('-n',namespace,'delete','--raw','/apis/apps/v1/namespaces/'+namespace+'/deployments/foreign-deletion-canary','-f','-',data=json.dumps({'apiVersion':'v1','kind':'DeleteOptions','propagationPolicy':'Foreground','preconditions':{'uid':foreign['metadata']['uid']}}))
 kub('-n',namespace,'wait','--for=delete','deployment/foreign-deletion-canary','--timeout=60s')
 # Repeat exact-value credential inspection AFTER restore and fresh API
 # transactions. Public evidence never includes the values used by this scan.
 private_values=list(tokens.values())+[unquote(urlsplit(env['DATABASE_URL']).password or '')]
 inventory=json.loads(kub('-n',namespace,'get','secrets','-o','json'))['items']
 def collect_credentials(value):
  if isinstance(value,dict):
   for key,value in value.items():
    if key in ('password','pass','secret') and isinstance(value,str) and value:private_values.append(value)
    collect_credentials(value)
  elif isinstance(value,list):
   for item in value:collect_credentials(item)
 for item in inventory:
  if item['metadata'].get('labels',{}).get('owner')=='helm':continue
  for encoded in item.get('data',{}).values():
   value=base64.b64decode(encoded).decode(errors='replace')
   if 'PRIVATE KEY' in value or re.fullmatch('[a-f0-9]{64}',value):private_values.append(value)
   try:collect_credentials(json.loads(value))
   except ValueError:pass
 private_values += [base64.b64encode(x.encode()).decode() for x in list(private_values) if x]
 def clean(text):return 'PRIVATE KEY' not in text and not any(x and x in text for x in private_values)
 if not clean(kub('-n',namespace,'get','fabriccas,fabricpeers,fabricorderernodes,fabricchaincodes,configmaps,pods,deployments','-o','json')):raise RuntimeError('Private material in restored public resources')
 histories=0
 for item in inventory:
  if item['metadata'].get('labels',{}).get('owner')!='helm':continue
  history=gzip.decompress(base64.b64decode(base64.b64decode(item['data']['release']))).decode()
  if not clean(history):raise RuntimeError('Private material in restored Helm history')
  histories+=1
 streams=0
 for pod in json.loads(kub('-n',namespace,'get','pods','-o','json'))['items']:
  for container in pod['spec']['containers']:
   if not clean(kub('-n',namespace,'logs',pod['metadata']['name'],'-c',container['name'])):raise RuntimeError('Private material in post-restore logs')
   streams+=1
 root=backend.parent
 names=run(['git','-C',root,'ls-files','--cached','--others','--exclude-standard','-z']).split('\0')
 paths=[root/name for name in names if name]+list(log.glob('*.json'))+list(log.glob('*.log'))+list((root/'fabric-console/dist').rglob('*'))
 files=0
 for path in paths:
  if not path.is_file():continue
  text=path.read_text(encoding='utf-8',errors='replace')
  if any(x and x in text for x in private_values):raise RuntimeError('Actual credential in post-restore source or evidence')
  files+=1
 report['checks']['preDeletionCredentialInspection']='PASS'
 if 'coldVolumeRecovery' in report:report['checks']['postColdCredentialInspection']='PASS'
 report['preDeletionCredentialInspection']={'status':'PASS','helmHistoryRecords':histories,'logStreams':streams,'sourceEvidenceFiles':files,'publicResourcesInspected':True}
 # Actual browser administrator preview/approval/deletion after recovery.
 payload={'tokens':tokens,'tenant':tenant,'project':'project-a','networkId':network,'operationId':current['operationId'],'result':str(log/'deletion-browser.json')}
 with (log/'deletion-browser.log').open('w') as output:
  p=subprocess.run(['node','tools/browser-deletion.mjs'],cwd=backend.parent/'fabric-console',input=json.dumps(payload),text=True,stdout=output,stderr=subprocess.STDOUT,timeout=420)
 if p.returncode:raise RuntimeError('Retained-data deletion browser acceptance failed')
 deletion=json.loads((log/'deletion-browser.json').read_text())
 actual={x['metadata']['name']:x['metadata']['uid'] for x in json.loads(kub('-n',namespace,'get','pvc','-o','json'))['items']}
 if actual!=volume_uids or json.loads(kub('-n',namespace,'get','deployments,jobs','-o','json'))['items']:raise RuntimeError('Retained deletion workload/storage observation failed')
 if any(x.get('status',{}).get('phase')!='Bound' for x in json.loads(kub('-n',namespace,'get','pvc','-o','json'))['items']):raise RuntimeError('Retained PVC is no longer bound')
 retained_digests={x['metadata']['name']:(x['metadata']['uid'],hashlib.sha256(json.dumps(x.get('data',{}),sort_keys=True).encode()).hexdigest()) for x in json.loads(kub('-n',namespace,'get','secrets','-o','json'))['items']}
 if retained_digests!=identity_digests:raise RuntimeError('Identity or Helm Secret bytes changed during retained deletion')
 report['checks']['retainedSecretBytesAndBoundPVCsVerified']='PASS'
 if json.loads(kub('-n',namespace,'get','namespace',namespace,'-o','json'))['metadata'].get('labels',{}).get('fabric.sathonate.io/retired','')=='':raise RuntimeError('Namespace retirement fence missing')
 if json.loads(kub('-n',namespace,'get','configmap','foreign-preservation-canary','-o','json'))['metadata']['uid']!=canary:raise RuntimeError('Foreign ConfigMap was altered by workload deletion')
 report['checks']['foreignWorkloadDeletionRefused']='PASS';report['checks']['foreignConfigMapPreserved']='PASS'
 report['checks']['previewedRetainedDataDeletion']='PASS';report['deletion']=deletion


def local_api(tokens):
 def api(path,token=tokens['primary'],method='GET',body=None,headers=None):
  data=None if body is None else json.dumps(body).encode()
  req=urllib.request.Request('http://127.0.0.1:8080/api/v1'+path,data=data,method=method,headers={'Authorization':'Bearer '+token,'Content-Type':'application/json',**(headers or {})})
  try:
   with urllib.request.urlopen(req,timeout=90) as res:return res.status,json.load(res)
  except urllib.error.HTTPError as e:return e.code,json.loads(e.read())
 return api
