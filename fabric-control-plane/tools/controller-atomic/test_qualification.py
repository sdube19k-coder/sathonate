"""Isolated harness tests; these are not Kubernetes/Fabric qualification."""
import argparse
import importlib.util
import unittest
from pathlib import Path
from unittest.mock import patch
from types import SimpleNamespace

spec = importlib.util.spec_from_file_location('atomic_qualification', Path(__file__).with_name('qualify.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class QualificationSafety(unittest.TestCase):
    def lab(self):
        return module.AtomicQualification(argparse.Namespace(cache='.', image='sathonate/controller-candidate:atomic'))

    def test_original_image_is_rejected_before_cluster_creation(self):
        lab = self.lab()
        lab.a.image = 'sathonate/controller-candidate:lab'
        with patch.object(module.Lifecycle, 'setup') as setup:
            with self.assertRaisesRegex(RuntimeError, 'Separate atomic image'):
                lab.setup()
            setup.assert_not_called()

    def test_admission_failure_is_scoped_to_exact_bundle_and_namespace(self):
        lab = self.lab()
        objects = []
        with patch.object(lab, 'guard') as guard, patch.object(lab, 'apply', side_effect=objects.append), patch.object(lab, 'until'):
            lab.admission('org1-peer-1-mat-test', 'secret', True)
        guard.assert_called_once()
        policy, binding = objects
        self.assertEqual(policy['spec']['matchConstraints']['resourceRules'][0]['operations'], ['CREATE'])
        self.assertEqual(policy['spec']['validations'][0]['expression'], "object.metadata.name != 'org1-peer-1-mat-test'")
        self.assertEqual(binding['spec']['matchResources']['namespaceSelector']['matchLabels'],
                         {'fabric.sathonate.io/tenant': module.NS})
        self.assertEqual(binding['spec']['validationActions'], ['Deny'])

    def test_unknown_failure_fixture_never_applied(self):
        lab = self.lab()
        with patch.object(lab, 'guard'), patch.object(lab, 'apply') as apply:
            with self.assertRaises(RuntimeError):
                lab.admission('reference', 'arbitrary', True)
            apply.assert_not_called()

    def test_incomplete_case_cannot_be_reported_as_qualified(self):
        lab = self.lab()
        lab.result['stages'] = {'baseline': 'PASS'}
        with patch.object(module.Lifecycle, 'lifecycle'), patch.object(lab, 'exec'), patch.object(lab, 'failure_case', side_effect=RuntimeError('injected')):
            with self.assertRaisesRegex(RuntimeError, 'injected'):
                lab.lifecycle()
        self.assertEqual(lab.result['status'], 'NOT_COMPLETED')
        self.assertNotEqual(lab.result['atomicQualification'], 'PASS')

    def test_wrong_live_certificate_prevents_transactions_and_success(self):
        lab = self.lab()
        with patch.object(lab, 'until'), patch.object(lab, 'kub'), \
                patch.object(lab, 'fingerprint', return_value='wrong'), patch.object(lab, 'transactions') as transactions:
            with self.assertRaisesRegex(RuntimeError, 'Live certificate mismatch'):
                lab.verify_recovered('reference', 'expected', 'test')
            transactions.assert_not_called()

    def test_operation_failure_exports_only_fixed_categories(self):
        lab = self.lab()
        canary = 'private-credential-canary-PRIVATE KEY-----'
        response = SimpleNamespace(returncode=1, stdout=canary, stderr='certificate rejected ' + canary)
        with patch.object(lab, 'kub', return_value=response):
            with self.assertRaises(RuntimeError) as caught:
                lab.exec('tools', 'peer', 'fixed-operation')
        self.assertNotIn(canary, str(caught.exception))
        self.assertNotIn(canary, str(lab.result))
        self.assertEqual(lab.result['operationFailure']['categories'], ['certificate'])

    def test_untrusted_transport_never_sends_transaction(self):
        lab = self.lab()
        response = SimpleNamespace(returncode=0, stdout='{"ready":false,"untrusted":true}')
        with patch.object(lab, 'kub', return_value=response), patch.object(module.Lifecycle, 'transactions') as transaction:
            with self.assertRaisesRegex(RuntimeError, 'TLS trust validation failed'):
                lab.transactions('untrusted')
            transaction.assert_not_called()

    def test_connection_warmup_requires_two_successful_probes_before_one_write(self):
        lab = self.lab()
        responses = iter([False, True, False, True, True])
        def wait(predicate, description, seconds):
            for _ in range(5):
                if predicate():return
            raise RuntimeError('not ready')
        with patch.object(lab, 'transport_ready', side_effect=lambda: next(responses)), \
                patch.object(lab, 'until', side_effect=wait), patch.object(module.Lifecycle, 'transactions') as transaction:
            lab.transactions('ready')
            transaction.assert_called_once_with('ready')

    def test_transport_script_is_valid_and_does_not_disable_verification(self):
        compile(module.TRANSPORT_PROBE, '<transport-probe>', 'exec')
        self.assertIn('ssl.create_default_context', module.TRANSPORT_PROBE)
        self.assertIn('server_hostname=', module.TRANSPORT_PROBE)
        self.assertNotIn('CERT_NONE', module.TRANSPORT_PROBE)


if __name__ == '__main__':
    unittest.main()
