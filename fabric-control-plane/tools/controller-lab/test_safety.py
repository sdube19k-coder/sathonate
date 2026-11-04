import importlib.util,tempfile,unittest,json
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch
spec=importlib.util.spec_from_file_location('lab_probe',Path(__file__).with_name('probe.py'))
probe=importlib.util.module_from_spec(spec);spec.loader.exec_module(probe)
build_spec=importlib.util.spec_from_file_location('lab_build',Path(__file__).with_name('build.py'))
build=importlib.util.module_from_spec(build_spec);build_spec.loader.exec_module(build)
class ControllerLabSafetyTests(unittest.TestCase):
 def test_explicit_context_and_cleanup_provider_guard(self):
  with tempfile.TemporaryDirectory() as tmp:
   args=SimpleNamespace(cache=tmp,kind='kind',source='reference',image='sathonate/controller-candidate:lab',output=str(Path(tmp)/'evidence.json'))
   p=probe.Probe(args)
   self.assertIn('--kubeconfig',p.base);self.assertIn('kind-'+p.name,p.base)
   p.created=True
   with patch.object(p,'kub',return_value=SimpleNamespace(stdout='kind://docker/unrelated/unrelated')),patch.object(p,'run') as command:
    with self.assertRaises(RuntimeError):p.finish()
    command.assert_not_called()
 def test_denial_requires_forbidden_not_missing_resource(self):
  with tempfile.TemporaryDirectory() as tmp:
   args=SimpleNamespace(cache=tmp,kind='kind',source='reference',image='image',output=str(Path(tmp)/'evidence.json'))
   p=probe.Probe(args)
   with patch.object(p,'kub',return_value=SimpleNamespace(returncode=1,stderr='NotFound')):
    with self.assertRaises(RuntimeError):p.denied('foreignSecret',['get','secret','canary'])
   self.assertEqual(p.result['checks']['foreignSecret'],'FAIL')
 def test_build_refuses_reference_and_nonempty_directories(self):
  with tempfile.TemporaryDirectory() as tmp:
   root=Path(tmp);source=root/'upstream';source.mkdir();candidate=root/'candidate';candidate.mkdir();(candidate/'user-work').write_text('preserve')
   with self.assertRaises(RuntimeError):build.prepare(source,source)
   with self.assertRaises(RuntimeError):build.prepare(source,candidate)
   self.assertEqual((candidate/'user-work').read_text(),'preserve')
 def test_source_drift_fails_without_overwriting(self):
  with tempfile.TemporaryDirectory() as tmp:
   p=Path(tmp)/'source';p.write_text('user work')
   with self.assertRaises(RuntimeError):build.replace(p,'missing','replacement')
   self.assertEqual(p.read_text(),'user work')
if __name__=='__main__':unittest.main()
