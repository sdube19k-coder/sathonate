import importlib.util, json, tempfile, unittest
from pathlib import Path
from unittest.mock import patch
spec=importlib.util.spec_from_file_location('evidence',Path(__file__).with_name('evidence.py'))
evidence=importlib.util.module_from_spec(spec);spec.loader.exec_module(evidence)
class EvidenceTests(unittest.TestCase):
 def test_redacted_manifest_and_skips(self):
  with tempfile.TemporaryDirectory() as tmp:
   root=Path(tmp)
   for name in ('cmd','internal','scripts','tools','.logs','examples'):(root/name).mkdir()
   for name in ('go.mod','go.sum'):(root/name).write_text('module sample')
   (root/'internal'/'source.go').write_text('package sample')
   (root/'examples'/'private-config.json').write_text('secret-do-not-export')
   (root/'.logs'/'controller-registry-kind.json').write_text(json.dumps({'status':'secret-do-not-export','checks':{'CARegistryPasswordsAbsentFromHelmHistory':'PASS','secret-do-not-export':'secret-do-not-export'},'registry':'secret-do-not-export'}))
   (root/'.logs'/'controller-lifecycle-kind.json').write_text(json.dumps({'status':'LOCAL_CANDIDATE_LIFECYCLE_PASS','fullFabricLifecycle':'PASS','stages':{'ca':'PASS','secret-do-not-export':'secret-do-not-export'},'checks':{'actualPublicResourcesContainNoCredentials':'PASS','secret-do-not-export':'secret-do-not-export'},'privateKey':'secret-do-not-export','candidateImage':'secret-do-not-export'}))
   (root/'.logs'/'controller-ca-image-evidence.json').write_text(json.dumps({'images':[{'imageID':'secret-do-not-export','token':'secret-do-not-export'}]}))
   (root/'.logs'/'controller-candidate-kind.json').write_text(json.dumps({'status':'secret-do-not-export','candidateImage':'secret-do-not-export','checks':{'foreignSecretCleanup':'PASS','secret-do-not-export':'secret-do-not-export'},'privateKey':'secret-do-not-export'}))
   (root/'.logs'/'controller-upstream-log-audit.json').write_text(json.dumps({'privateKeyMarkerObservedInSample':True,'privateKey':'secret-do-not-export'}))
   (root/'.logs'/'milestone-stored-cr-inspection.json').write_text(json.dumps({'passwordFree':False,'resources':[{'privateKey':'secret-do-not-export'}],'connection':'secret-do-not-export'}))
   (root/'.logs'/'enterprise-final-tests.log').write_text('secret-do-not-export\n--- SKIP: TestOptional (0s)\n--- PASS: TestSafe (0s)\n')
   (root/'.logs'/'controller-lifecycle-safety.log').write_text('secret-do-not-export\nRan 2 tests in 0.010s\n\nOK (skipped=1)\n')
   (root/'.logs'/'controller-registry-kind-clientconfig-failure.log').write_text('Traceback (most recent call last):\nRuntimeError: secret-do-not-export\n')
   with patch.object(evidence,'ROOT',root),patch.object(evidence,'run',return_value='secret-do-not-export'):
    report=evidence.collect()
   self.assertNotIn('secret-do-not-export',json.dumps(report))
   self.assertIsNone(report['sourceRevision']);self.assertFalse(report['sourceClean']);self.assertFalse(report['awsQualified'])
   self.assertEqual(report['testLogs']['enterprise-final-tests.log']['cases']['TestOptional'],'SKIP')
   self.assertEqual(report['testLogs']['controller-lifecycle-safety.log']['unittest'],{'count':2,'skipped':1,'status':'PASS'})
   self.assertEqual(report['images'],{})
   self.assertEqual(report['controllerCandidateProbe']['fullFabricLifecycle'],'BLOCKED')
   self.assertEqual(report['controllerCandidateProbe']['checks'],{'foreignSecretCleanup':'PASS'})
   self.assertEqual(report['controllerCandidateProbe']['status'],'NOT VERIFIED')
   self.assertEqual(report['protectedCARegistryProbe']['checks'],{'CARegistryPasswordsAbsentFromHelmHistory':'PASS'})
   self.assertTrue(report['testLogs']['controller-registry-kind-clientconfig-failure.log']['runtimeFailure'])
   self.assertEqual(report['actualCAProbeImages']['imageIDs'],[])
   self.assertEqual(report['secureControllerLifecycle']['fullFabricLifecycle'],'BLOCKED')
   self.assertFalse(report['secureControllerLifecycle']['milestone2Complete'])
   self.assertEqual(report['secureControllerLifecycle']['checks'],{'actualPublicResourcesContainNoCredentials':'PASS'})
 def test_stage_claims_without_runtime_security_checks_do_not_qualify(self):
  with tempfile.TemporaryDirectory() as tmp:
   root=Path(tmp)
   for name in ('cmd','internal','scripts','tools','.logs'):(root/name).mkdir()
   for name in ('go.mod','go.sum'):(root/name).write_text('module fixture')
   data={'status':'LOCAL_CANDIDATE_LIFECYCLE_PASS','disposableClusterRemoved':True,'stages':{name:'PASS' for name in ('ca','identities','orderers','peers','channel','chaincode','recovery','rotation','security')},'checks':{'committedTransactions-initial':'PASS'},'runtimeImageDigests':['private-canary','sha256:'+'a'*64]}
   (root/'.logs/controller-lifecycle-kind.json').write_text(json.dumps(data))
   with patch.object(evidence,'ROOT',root),patch.object(evidence,'run',return_value=None):report=evidence.collect()
   self.assertEqual(report['secureControllerLifecycle']['fullFabricLifecycle'],'BLOCKED')
   self.assertEqual(report['secureControllerLifecycle']['runtimeImageDigests'],['sha256:'+'a'*64])
   self.assertNotIn('private-canary',json.dumps(report))
 def test_source_content_hash_changes(self):
  with tempfile.TemporaryDirectory() as tmp:
   root=Path(tmp)
   for name in ('cmd','internal','scripts','tools'):(root/name).mkdir()
   for name in ('go.mod','go.sum'):(root/name).write_text('a')
   with patch.object(evidence,'ROOT',root),patch.object(evidence,'run',return_value=None):
    first=evidence.collect()['sourceContentSHA256'];(root/'go.mod').write_text('b');second=evidence.collect()['sourceContentSHA256']
   self.assertNotEqual(first,second)
if __name__=='__main__':unittest.main()
