import importlib.util
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('sprint2_evidence', Path(__file__).with_name('evidence.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class EvidenceSafety(unittest.TestCase):
    def test_skipped_runtime_is_never_pass(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(module, 'LOG', Path(directory)):
            path = Path(directory) / 'tests.jsonl'
            path.write_text('{"Action":"skip","Test":"Runtime"}\n')
            result = module.structured_go(path.name)
            self.assertNotEqual(result['status'], 'PASS')
            self.assertEqual(result['skipped'], ['Runtime'])

    def test_package_failure_overrides_passing_case_and_withholds_output(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(module, 'LOG', Path(directory)):
            path = Path(directory) / 'tests.jsonl'
            path.write_text('{"Action":"pass","Test":"Case"}\n{"Action":"output","Output":"private-diagnostic-canary"}\n{"Action":"fail","Package":"fixture"}\n')
            result = module.structured_go(path.name)
            self.assertEqual(result['status'], 'FAIL')
            self.assertNotIn('private-diagnostic-canary', str(result))

    def test_truncated_go_log_cannot_pass(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(module, 'LOG', Path(directory)):
            path=Path(directory)/'tests.jsonl'
            path.write_text('{"Action":"start","Package":"fixture"}\n{"Action":"pass","Test":"Case","Package":"fixture"}\n')
            result=module.structured_go(path.name)
            self.assertEqual(result['status'],'NOT VERIFIED')
            self.assertEqual(result['incompletePackages'],['fixture'])

    def test_missing_evidence_is_explicit(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(module, 'LOG', Path(directory)):
            self.assertEqual(module.structured_go('missing')['status'], 'NOT VERIFIED')
            self.assertEqual(module.unittest('missing')['status'], 'NOT VERIFIED')

    def test_unittest_failure_does_not_become_pass(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(module, 'LOG', Path(directory)):
            path = Path(directory) / 'tests.log'
            path.write_text('Ran 4 tests in 0.01s\nFAILED (failures=1)\n')
            self.assertEqual(module.unittest(path.name)['status'], 'FAIL')


if __name__ == '__main__':
    unittest.main()
