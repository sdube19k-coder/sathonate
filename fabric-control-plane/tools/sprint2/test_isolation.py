"""Fixture safety only; these tests never count as network enforcement proof."""
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('sprint2_isolation', Path(__file__).with_name('isolation.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class FixtureSafety(unittest.TestCase):
    def test_privileged_denial_reaches_admission_not_spec_contradiction(self):
        class Observed(Exception):
            pass

        def denied(manifest, marker):
            security = manifest['spec']['containers'][0]['securityContext']
            self.assertTrue(security['privileged'])
            self.assertNotEqual(security.get('allowPrivilegeEscalation'), False)
            self.assertEqual(marker, 'violates PodSecurity')
            raise Observed()

        with self.assertRaises(Observed):
            module.inspect(None, None, denied, 'fixture', 'approved-image', {'cluster': 'fixture'})


if __name__ == '__main__':
    unittest.main()
