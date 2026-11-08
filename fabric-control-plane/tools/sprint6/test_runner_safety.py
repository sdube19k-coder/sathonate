import ast,importlib.util,json,re,types,unittest
from pathlib import Path
from worker_failure import Failure,completed_case,cross_probe_workers,OUTAGE,RECOVERY,CLEANUP

class RunnerSafety(unittest.TestCase):
 def fixture(self,multi,foreign=False):
  source=Path(__file__).resolve().parents[1]/'sprint1/qualify.py';tree=ast.parse(source.read_text(encoding='utf-8'))
  functions=[n for n in tree.body if isinstance(n,ast.FunctionDef) and n.name in ('image_nodes','guard_engine','guard','cleanup_cluster')]
  cluster='smp-sprint1-a1b2c3d4';calls=[];nodes=[cluster+'-control-plane']+([cluster+'-worker',cluster+'-worker2',cluster+'-worker3'] if multi else [])
  def run(args,**kwargs):
   args=list(map(str,args));calls.append(args)
   if args[:3]==['kind','get','nodes']:return '\n'.join(nodes)
   if args[:2]==['docker','inspect']:
    return json.dumps({'id':'a'*64,'labels':{'io.x-k8s.kind.cluster':'smp-qualification' if foreign else cluster},'paused':True})
   return ''
  def kub(*args,**kwargs):return 'kind://docker/'+cluster+'/'+args[2]
  ns={'NAME':cluster,'MULTI':multi,'KIND':Path('kind'),'run':run,'kub':kub,'json':json,'re':re,'preflight':types.SimpleNamespace(local_engine_endpoint=lambda:'local'),'report':{'dockerEndpoint':'local'}}
  exec(compile(ast.Module(body=functions,type_ignores=[]),str(source),'exec'),ns);return ns,calls
 def test_cleanup_checks_all_ownership_before_unpause_or_delete(self):
  ns,calls=self.fixture(True,True)
  with self.assertRaises(RuntimeError):ns['cleanup_cluster']()
  self.assertFalse(any('unpause' in c or 'delete' in c for c in calls))
 def test_owned_paused_nodes_are_unpaused_before_kind_teardown(self):
  ns,calls=self.fixture(True);ns['cleanup_cluster']()
  self.assertEqual(calls[-1],['kind','delete','cluster','--name',ns['NAME']])
  self.assertEqual(sum('unpause' in c for c in calls),4)
 def test_single_node_cleanup_preserves_the_existing_provider_guard(self):
  ns,calls=self.fixture(False);ns['cleanup_cluster']();self.assertEqual(calls,[['kind','delete','cluster','--name',ns['NAME']]])
 def test_pass_requires_every_runtime_and_cleanup_gate(self):
  value={'status':'PASS_BOUNDED_WORKER_PAUSE_RECOVERY','checks':dict.fromkeys(['authorization']+OUTAGE+RECOVERY,True),'cleanup':dict.fromkeys(CLEANUP,True)}
  self.assertTrue(completed_case(value))
  for key in value['checks']:
   value['checks'][key]=False;self.assertFalse(completed_case(value));value['checks'][key]=True
  for key in value['cleanup']:
   value['cleanup'][key]=False;self.assertFalse(completed_case(value));value['cleanup'][key]=True
  self.assertFalse(completed_case({}))
 def test_probe_pair_exercises_three_different_workers_including_peer(self):
  workers=['owned-a','owned-b','owned-c'];self.assertEqual(cross_probe_workers(workers,'owned-b'),['owned-a','owned-c'])
  with self.assertRaises(Failure):cross_probe_workers(workers,'retained-node')

class CrossWorkerIsolationFixture(unittest.TestCase):
 def test_explicit_worker_placement_and_denial_controls(self):
  source=Path(__file__).resolve().parents[1]/'sprint2/isolation.py';spec=importlib.util.spec_from_file_location('isolation_s6',source);module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
  applied=[];ns='owned';workers=['worker-a','worker-b'];report={'cluster':'fresh','checks':{},'networkPolicyFixture':{}}
  def kub(*args,**kwargs):
   if 'exec' in args:return 'DENIED' if 'DENIED' in args[-1] else 'CONNECTED'
   if 'wait' in args:return ''
   if 'get' in args and 'serviceaccount' in args:return json.dumps({'automountServiceAccountToken':False})
   if 'get' in args and 'pods' in args:return json.dumps({'items':[]})
   if 'jsonpath={.status.podIP}' in args:return '10.0.0.5'
   if 'get' in args and 'pod' in args:return json.dumps({'spec':{'nodeName':workers[0] if args[1]==ns else workers[1]}})
   raise AssertionError('unexpected fixture command')
  module.inspect(kub,applied.append,lambda *args:None,ns,'approved-image',report,probe_nodes=workers)
  self.assertEqual(report['checks']['crossWorkerCalicoEnforcement'],'PASS')
  for pod in applied:self.assertEqual(pod['spec']['nodeSelector']['kubernetes.io/hostname'],workers[0] if pod['metadata']['namespace']==ns else workers[1])
  # This is a mocked fixture regression, not evidence of live CNI enforcement.

if __name__=='__main__':unittest.main()
