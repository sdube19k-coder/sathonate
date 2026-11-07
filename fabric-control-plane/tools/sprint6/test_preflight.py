import copy,unittest
from preflight import assess,cluster_plan,worker_inventory,local_endpoint,GIB
class Preflight(unittest.TestCase):
 def good(self):return {'dockerCPUs':4,'dockerMemoryTotal':8*GIB,'dockerMemoryAvailable':6*GIB,'hostMemoryAvailable':2*GIB,'backingDiskFree':40*GIB}
 def test_exact_budget_is_preflight_only(self):self.assertEqual(assess(self.good())['status'],'PASS_PREFLIGHT_ONLY')
 def test_each_resource_is_required_and_fail_closed(self):
  for field in self.good():
   for value in [None,'unknown',-1,True,self.good()[field]-1]:
    with self.subTest(field=field,value=value):
     m=self.good();m[field]=value;self.assertEqual(assess(m)['status'],'BLOCKED_CAPACITY')
 def test_check_does_not_mutate_input(self):
  m=self.good();before=copy.deepcopy(m);assess(m);self.assertEqual(m,before)
 def test_four_distinct_kind_nodes_and_worker_only_inventory(self):
  name='smp-sprint1-a1b2c3d4';p=cluster_plan(name);i=worker_inventory(name)
  self.assertEqual([n['role'] for n in p['nodes']],['control-plane','worker','worker','worker']);self.assertTrue(p['networking']['disableDefaultCNI'])
  self.assertEqual(len(set(i['workerNodes'])),3)
  self.assertNotIn(name+'-control-plane',str(i['nodeSelectors']))
 def test_remote_docker_capacity_is_never_inferred(self):
  self.assertTrue(local_endpoint('npipe:////./pipe/dockerDesktopLinuxEngine','nt'));self.assertTrue(local_endpoint('unix:///var/run/docker.sock','posix'))
  for endpoint in ['tcp://remote:2376','ssh://remote','https://remote','unix:///var/run/docker.sock']:
   self.assertFalse(local_endpoint(endpoint,'nt'))
 def test_retained_or_ambiguous_name_refused(self):
  for name in ['smp-qualification','smp-sprint1-a1b2c3d4;bad','other','smp-sprint1-']:
   with self.assertRaises(ValueError):cluster_plan(name)
if __name__=='__main__':unittest.main()
