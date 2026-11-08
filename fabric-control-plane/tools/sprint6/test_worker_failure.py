import copy,io,json,os,secrets,socket,tempfile,threading,unittest
from pathlib import Path
import urllib.error
from unittest.mock import patch
from worker_failure import Failure,candidates,pv_host,unchanged_storage,run_case,OUTAGE,RECOVERY,CLEANUP
from worker_live import API,Live,valid_transaction,listener_owned,redact_log,qualify,require_disposable_database

class Model:
 def __init__(self,fail=None):self.fail=fail;self.events=[];self.paused=False
 def event(self,name):
  self.events.append(name)
  if self.fail==name:raise Failure('injected-'+name)
 def prepare(self):self.event('prepare');return {'worker':'owned-worker'}
 def authorize(self):self.event('authorize');return {'authorization':True}
 def pause(self,worker):self.paused=True;self.event('pause')
 def observe_outage(self,worker):self.event('outage');return dict.fromkeys(OUTAGE,True)
 def resume(self,worker):self.paused=False;self.event('resume')
 def recover(self,worker):self.event('recover');return {'checks':dict.fromkeys(RECOVERY,True),'evidence':{'public':'receipt'}}
 def cleanup(self):self.event('cleanup');return {**dict.fromkeys(CLEANUP,True),'workerUnpaused':not self.paused}

class StateMachine(unittest.TestCase):
 def run_model(self,model):
  saved=[];ticks=iter(range(100,200));return run_case(model,saved.append,lambda:next(ticks)),saved
 def test_complete_case_and_separate_timings(self):
  model=Model();receipt,saved=self.run_model(model)
  self.assertEqual(receipt['status'],'PASS_BOUNDED_WORKER_PAUSE_RECOVERY');self.assertFalse(model.paused)
  self.assertEqual(receipt['durationsSeconds'],{'outageObservation':1,'recoveryIncludingNewTransactions':1})
  self.assertEqual(saved[0]['stage'],'PREFLIGHT_COMPLETE');self.assertEqual(saved[0]['status'],'IN_PROGRESS');self.assertEqual(saved[-1]['stage'],'FINISHED')
 def test_failure_at_each_boundary_cleans_up_and_never_passes(self):
  for stage in ['prepare','authorize','pause','outage','resume','recover','cleanup']:
   with self.subTest(stage=stage):
    model=Model(stage);receipt,_=self.run_model(model);self.assertEqual(receipt['status'],'FAIL');self.assertIn('cleanup',model.events)
    if stage!='cleanup':self.assertEqual(receipt['failedBoundary'],'authorization' if stage=='authorize' else ('observe_outage' if stage=='outage' else stage))
    if stage not in ['prepare','authorize']:self.assertIn('resume',model.events)
 def test_side_effect_before_pause_ack_is_always_reversed(self):
  model=Model('pause');receipt,_=self.run_model(model);self.assertFalse(model.paused);self.assertEqual(receipt['cleanupResume'],'ACKNOWLEDGED')
 def test_missing_or_false_security_check_prevents_pass(self):
  for check in OUTAGE:
   model=Model();model.observe_outage=lambda worker,check=check:{k:k!=check for k in OUTAGE}
   receipt,_=self.run_model(model);self.assertEqual(receipt['status'],'FAIL');self.assertFalse(model.paused)
  for check in RECOVERY:
   model=Model();model.recover=lambda worker,check=check:{'checks':{k:k!=check for k in RECOVERY},'evidence':{}}
   self.assertEqual(self.run_model(model)[0]['status'],'FAIL')
 def test_cleanup_failure_cannot_overwrite_failure_as_success(self):
  model=Model();model.cleanup=lambda:{'workerUnpaused':True,'uidCheckedResourcesRemoved':False,'privateGrantsRemoved':True}
  self.assertEqual(self.run_model(model)[0]['status'],'FAIL')
 def test_private_exception_text_is_not_evidence(self):
  secret=secrets.token_hex(32);model=Model();model.prepare=lambda:(_ for _ in ()).throw(ValueError(secret))
  receipt,saved=self.run_model(model);self.assertNotIn(secret,json.dumps(saved));self.assertEqual(receipt['failureCategory'],'ValueError')

class Validation(unittest.TestCase):
 def topology(self):
  workers=['owned-a','owned-b','owned-c'];nodes=[{'id':'order-'+str(i),'role':'orderer'} for i in range(5)]+[{'id':'peer-'+str(i),'role':'peer'} for i in range(3)]
  authorities=[{'id':'authority-'+str(i)} for i in range(3)]
  channels=[{'id':name,'ordererRefs':['order-'+str(i) for i in range(5)]} for name in ['customer-channel','second-channel']]
  network={'spec':{'nodes':nodes,'channels':channels,'configuration':{'authorities':authorities}}}
  components={n['id']:{'node':workers[i%3]} for i,n in enumerate(nodes[:5])}
  components.update({'peer-'+str(i):{'node':workers[i]} for i in range(3)});components.update({'authority-'+str(i):{'node':workers[i]} for i in range(3)})
  return network,workers,components
 def test_generic_worker_choices_preserve_all_channel_majorities(self):
  network,workers,components=self.topology();result=candidates(network,workers,components)
  self.assertEqual(len(result),3)
  self.assertTrue(all(len(b['remaining'])>=b['required'] for c in result for b in c['channels']))
 def test_missing_members_or_lost_quorum_cannot_authorize_pause(self):
  network,workers,components=self.topology()
  for value in components.values():value['node']=workers[0]
  with self.assertRaises(Failure):candidates(network,workers,components)
  network,workers,components=self.topology();components.pop('order-0')
  with self.assertRaises(Failure):candidates(network,workers,components)
 def test_separate_channels_are_evaluated_independently(self):
  network,workers,components=self.topology();network['spec']['channels'][1]['ordererRefs']=['order-0','order-1','order-2']
  result=candidates(network,workers,components);self.assertEqual(len(result),3)
 def test_no_channel_is_never_a_fabric_acceptance(self):
  network,workers,components=self.topology();network['spec']['channels']=[]
  with self.assertRaises(Failure):candidates(network,workers,components)
 def affinity(self,hosts):return {'required':{'nodeSelectorTerms':[{'matchExpressions':[{'key':'kubernetes.io/hostname','operator':'In','values':[h]}]} for h in hosts]}}
 def test_local_pv_cannot_claim_migration_or_ambiguous_affinity(self):
  self.assertEqual(pv_host(self.affinity(['owned-a'])),'owned-a')
  for value in [{},self.affinity(['owned-a','owned-b']),{'required':{'nodeSelectorTerms':[{'matchExpressions':[]}]}}]:
   with self.assertRaises(Failure):pv_host(value)
 def test_original_pvc_pv_binding_and_host_are_all_required(self):
  before={'data':{'uid':'pvc','pv':'volume','pvUID':'uid','host':'owned-a','phase':'Bound'}}
  unchanged_storage(before,copy.deepcopy(before))
  for key in ['uid','pv','pvUID','host','phase']:
   after=copy.deepcopy(before);after['data'][key]='changed'
   with self.assertRaises(Failure):unchanged_storage(before,after)
  with self.assertRaises(Failure):unchanged_storage(before,{})
 def test_prior_transaction_is_valid_and_framing_is_bounded(self):
  tx=secrets.token_hex(32);raw=b'\x0a\x40'+tx.encode();valid_transaction(raw,tx)
  for bad in [b'',raw+b'\x10\x0b',b'\x0a\xff'+tx.encode(),b'\x00'+tx.encode()]:
   with self.assertRaises(Failure):valid_transaction(bad,tx)

class AdapterGuards(unittest.TestCase):
 def adapter(self):
  obj=object.__new__(Live);obj.cluster='smp-sprint1-a1b2c3d4';obj.worker=obj.cluster+'-worker';obj.target_id='a'*64;obj.ns='owned-namespace';obj.guard=lambda:None;obj.resume_lock=threading.RLock();obj.watchdog=None;obj.watchdog_fired=False;obj.watchdog_error=False
  return obj
 def test_native_api_cannot_open_a_remote_or_nonfixture_database(self):
  require_disposable_database('postgres://127.0.0.1/fabric_smp_tests?sslmode=disable')
  for url in ['postgres://cloud/fabric_smp_tests','postgres://127.0.0.1/production','postgres://127.0.0.1/fabric_smp_tests/other','http://127.0.0.1/fabric_smp_tests','postgres://127.0.0.1/fabric_smp_tests#ambiguous','']:
   with self.assertRaises(Failure):require_disposable_database(url)
 def test_capacity_refusal_precedes_native_api_or_resource_creation(self):
  with patch('worker_live.Live') as adapter:
   with self.assertRaises(Failure):qualify(None,None,None,None,None,None,None,None,None,{'capacity':{'status':'BLOCKED_CAPACITY'}})
   adapter.assert_not_called()
 def test_replaced_resource_is_never_deleted(self):
  obj=self.adapter();calls=[]
  def kub(*args,**kwargs):calls.append(args);return json.dumps({'metadata':{'uid':'foreign'}})
  obj.kub=kub
  with self.assertRaises(Failure):obj.delete({'kind':'Pod','name':'probe','uid':'original'})
  self.assertTrue(all('delete' not in c for c in calls))
 def test_unacknowledged_create_never_adopts_a_same_name_resource(self):
  obj=self.adapter();obj.created=[]
  obj.kub=lambda *args,**kwargs:(_ for _ in ()).throw(OSError('lost acknowledgement'))
  with self.assertRaises(OSError):obj.create('Pod','probe',{})
  self.assertEqual(obj.created,[{'kind':'Pod','name':'probe','uid':None}])
  obj.kub=lambda *args,**kwargs:json.dumps({'metadata':{'uid':'unknown'}})
  with self.assertRaises(Failure):obj.delete(obj.created[0])
 def test_cleanup_has_exact_uid_and_never_force_or_pvc_delete(self):
  obj=self.adapter();calls=[]
  def kub(*args,**kwargs):
   calls.append((args,kwargs));return json.dumps({'metadata':{'uid':'original'}}) if 'get' in args else ''
  obj.kub=kub;obj.delete({'kind':'Pod','name':'probe','uid':'original'})
  delete=next((a,k) for a,k in calls if a[0]=='delete');self.assertTrue(delete[0][2].endswith('/pods/probe'));self.assertEqual(json.loads(delete[1]['data'])['preconditions'],{'uid':'original'})
  self.assertNotIn('--force',str(calls));self.assertNotIn('persistentvolumeclaims',str(calls))
 def test_docker_control_plane_or_replaced_id_is_not_resumed(self):
  for mutation in ['role','id','name','cluster']:
   obj=self.adapter();calls=[];info={'id':'a'*64,'name':'/'+obj.worker,'labels':{'io.x-k8s.kind.cluster':obj.cluster,'io.x-k8s.kind.role':'worker'},'running':True,'paused':True}
   if mutation=='role':info['labels']['io.x-k8s.kind.role']='control-plane'
   if mutation=='id':info['id']='b'*64
   if mutation=='name':info['name']='/smp-qualification-control-plane'
   if mutation=='cluster':info['labels']['io.x-k8s.kind.cluster']='smp-qualification'
   obj.run=lambda args,**kwargs:(calls.append(args) or json.dumps(info))
   with self.assertRaises(Failure):obj.resume(obj.worker)
   self.assertFalse(any('unpause' in c for c in calls))
 def test_pause_watchdog_restores_only_owned_worker_and_cannot_claim_success(self):
  obj=self.adapter();calls=[];info={'id':'a'*64,'name':'/'+obj.worker,'labels':{'io.x-k8s.kind.cluster':obj.cluster,'io.x-k8s.kind.role':'worker'},'running':True,'paused':True}
  def run(args,**kwargs):
   calls.append(args)
   if 'unpause' in args:info['paused']=False;return ''
   return json.dumps(info)
  obj.run=run;obj.expire_pause();self.assertTrue(obj.watchdog_fired);self.assertFalse(obj.watchdog_error);self.assertFalse(info['paused'])
  self.assertEqual([c for c in calls if 'unpause' in c],[['docker','unpause','a'*64]])
 def test_resume_is_idempotent_without_restarting_an_unpaused_worker(self):
  obj=self.adapter();calls=[];info={'id':'a'*64,'name':'/'+obj.worker,'labels':{'io.x-k8s.kind.cluster':obj.cluster,'io.x-k8s.kind.role':'worker'},'running':True,'paused':False}
  obj.run=lambda args,**kwargs:(calls.append(args) or json.dumps(info));obj.resume(obj.worker)
  self.assertFalse(any('unpause' in c for c in calls))
 @unittest.skipUnless(os.name=='nt','native Windows listener attestation')
 def test_actual_loopback_listener_must_belong_to_the_expected_process(self):
  with socket.socket() as sock:
   sock.bind(('127.0.0.1',0));sock.listen();port=sock.getsockname()[1]
   self.assertTrue(listener_owned(port,os.getpid()))
   self.assertFalse(listener_owned(port,os.getpid()+1))
 def test_listener_port_cannot_inject_shell_arguments(self):
  with self.assertRaises(Failure):listener_owned('1;bad',1)
 def test_private_grants_are_removed_even_when_native_process_stop_fails(self):
  obj=self.adapter();obj.target_id=None;obj.created=[];obj.log_exposure=False;obj.log_stream_failed=False;obj.log_thread=None;obj.log_handle=None
  class Process:
   stdout=None
   def poll(self):return None
   def terminate(self):raise OSError('stop failed')
  obj.process=Process()
  with tempfile.TemporaryDirectory() as directory:
   obj.private=Path(directory)/'owned';obj.private.mkdir();(obj.private/'grants.json').write_text('hashed-only');obj.grant_created=True
   value=obj.cleanup();self.assertTrue(value['privateGrantsRemoved']);self.assertFalse(value['apiStopped']);self.assertFalse(obj.private.exists())
 def test_probe_is_tokenless_nonroot_and_has_no_writable_pvc(self):
  obj=self.adapter();obj.config={'workerImage':'example/worker@sha256:'+'a'*64};pod=obj.pod_spec()
  self.assertFalse(pod['automountServiceAccountToken']);self.assertTrue(pod['securityContext']['runAsNonRoot'])
  self.assertTrue(pod['containers'][0]['securityContext']['readOnlyRootFilesystem']);self.assertNotIn('volumes',pod)
 def test_plain_authentication_denial_stays_401_and_missing_token_is_missing(self):
  token=secrets.token_hex(32);api=API(12345,{'primary':token})
  failure=urllib.error.HTTPError('http://127.0.0.1',401,'Unauthorized',{},io.BytesIO(b'authentication required'))
  with patch('worker_live.urllib.request.urlopen',side_effect=failure) as opened:
   status,_=api.request('/projects',None)
  self.assertEqual(status,401);self.assertIsNone(opened.call_args.args[0].get_header('Authorization'))
 def test_native_log_is_redacted_before_disk_and_exposure_is_not_hidden(self):
  secret=secrets.token_hex(32);output=io.StringIO();marked=[]
  lines=['normal startup\n','credential '+secret+'\n','-----BEGIN PRIVATE KEY-----\n','privatebase64line\n','-----END PRIVATE KEY-----\n','finished\n']
  redact_log(lines,output,[secret],lambda:marked.append(True));value=output.getvalue()
  self.assertTrue(marked);self.assertNotIn(secret,value);self.assertNotIn('privatebase64line',value);self.assertNotIn('PRIVATE KEY',value);self.assertIn('finished',value)
 def test_no_token_value_can_enter_api_evidence(self):
  token=secrets.token_hex(32);api=API(12345,{'primary':token})
  class Response(io.BytesIO):status=200
  with patch('worker_live.urllib.request.urlopen',return_value=Response(json.dumps({'unexpected':token}).encode())):
   with self.assertRaises(Failure):api.request('/projects')

if __name__=='__main__':unittest.main()
