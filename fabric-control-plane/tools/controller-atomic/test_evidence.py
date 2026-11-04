import importlib.util
import json
import unittest
from pathlib import Path

spec = importlib.util.spec_from_file_location('atomic_evidence', Path(__file__).with_name('evidence.py'))
evidence = importlib.util.module_from_spec(spec)
spec.loader.exec_module(evidence)


class EvidenceBoundary(unittest.TestCase):
    def test_failed_runtime_is_reported_as_failed(self):
        self.assertEqual(evidence.summarize({'atomicQualification': 'FAIL'})['qualification'], 'FAIL')

    def test_untrusted_fields_and_sensitive_values_are_not_exported(self):
        canary = 'credential-canary-PRIVATE KEY-----'
        data = {'candidateImage': canary, 'password': canary, 'baseline': {'checks': {canary: 'PASS'}},
                'failureCases': {'before-secret': {'status': canary, 'liveCertificateSHA256': canary,
                    'password': canary, 'logs': canary}, canary: {'status': 'PASS'}}}
        result = evidence.summarize(data)
        self.assertNotIn(canary, json.dumps(result))
        self.assertEqual(result['qualification'], 'NOT VERIFIED')
        self.assertFalse(result['awsQualified'])

    def test_declared_success_without_runtime_cases_is_not_qualification(self):
        result = evidence.summarize({'status': 'LOCAL_ATOMIC_QUALIFICATION_PASS', 'atomicQualification': 'PASS',
                                     'disposableClusterRemoved': True})
        self.assertEqual(result['qualification'], 'NOT VERIFIED')

    def test_all_cases_baseline_and_cleanup_are_required(self):
        fields = {'status': 'PASS', 'recovered': True, 'immutableBundleReused': True,
                  'previousBundleRetained': True, 'committedAndQueriedOnBothPeers': True,
                  'previousNetworkOperational': True}
        data = {'status': 'LOCAL_ATOMIC_QUALIFICATION_PASS', 'atomicQualification': 'PASS',
                'disposableClusterRemoved': True,
                'baseline': {'stages': dict.fromkeys(evidence.STAGES, 'PASS'),
                             'checks': dict.fromkeys(range(42), 'PASS')},
                'failureCases': {point: dict(fields) for point in evidence.POINTS}}
        self.assertEqual(evidence.summarize(data)['qualification'], 'PASS')
        data['disposableClusterRemoved'] = False
        self.assertEqual(evidence.summarize(data)['qualification'], 'NOT VERIFIED')


if __name__ == '__main__':
    unittest.main()
