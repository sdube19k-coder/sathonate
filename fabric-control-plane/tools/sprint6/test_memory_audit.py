import unittest
from memory_audit import configured_memory
from preflight import assess,GIB
class MemoryAudit(unittest.TestCase):
 def test_explicit_owner_limit_is_data_not_an_instruction(self):
  value=configured_memory('[wsl2]\n# do not execute this comment\nmemory=6GB\n');self.assertEqual(value['bytes'],6*GIB);self.assertEqual(value['state'],'EXPLICIT')
 def test_no_default_or_ambiguous_limit_is_invented(self):
  for text in ['', '[wsl2]\nprocessors=4\n','[wsl2]\nmemory=anything\n','invalid text']:
   self.assertNotIn('bytes',configured_memory(text))
 def test_docker_budget_alone_is_not_physical_host_headroom(self):
  good={'dockerCPUs':20,'dockerMemoryTotal':9*GIB,'dockerMemoryAvailable':6*GIB,'hostMemoryAvailable':8*GIB,'backingDiskFree':50*GIB}
  self.assertEqual(assess(good)['status'],'PASS_PREFLIGHT_ONLY');good['hostMemoryAvailable']=4*GIB
  value=assess(good);self.assertEqual(value['status'],'BLOCKED_CAPACITY');self.assertEqual(value['failures'][0]['metric'],'hostMemoryAvailable')
if __name__=='__main__':unittest.main()
