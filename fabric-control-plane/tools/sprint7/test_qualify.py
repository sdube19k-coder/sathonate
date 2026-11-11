import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import qualify


class ConsolidatedAcceptance(unittest.TestCase):
    def test_blocked_capacity_prevents_dispatch(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / 'receipt.json'
            argv = ['qualify.py', '--run', '--output', str(output)]
            with patch.object(qualify.sys, 'argv', argv), patch.object(qualify, 'preflight', return_value={'status': 'BLOCKED_CAPACITY'}), patch.object(qualify, 'execute') as execute:
                self.assertEqual(qualify.main(), 2)
                execute.assert_not_called()
            receipt = json.loads(output.read_text())
            self.assertEqual(receipt['liveAcceptance'], 'NOT_TESTED')
            self.assertFalse(receipt['fixtureInvoked'])

    def test_receipt_cannot_pass_without_restore_browser_and_cleanup(self):
        receipt = {'status': 'PASS', 'disposableClusterRemoved': True, 'fixtureDatabaseScopeRemoved': True,
                   'retainedNetwork': {'ready': True},
                   'checks': dict.fromkeys(qualify.REQUIRED_CHECKS, 'PASS'),
                   'browser': {'checks': dict.fromkeys(qualify.BROWSER_CHECKS, 'PASS')}}
        self.assertTrue(qualify.validate_receipt(receipt))
        for key in ('disposableClusterRemoved', 'fixtureDatabaseScopeRemoved'):
            self.assertFalse(qualify.validate_receipt({**receipt, key: False}))
        for key in qualify.REQUIRED_CHECKS:
            self.assertFalse(qualify.validate_receipt({**receipt, 'checks': {**receipt['checks'], key: 'FAIL'}}))
        self.assertFalse(qualify.validate_receipt({**receipt, 'browser': {'checks': {}}}))

    def test_existing_evidence_is_never_replaced(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / 'receipt.json'
            output.write_text('existing evidence')
            with patch.object(qualify.sys, 'argv', ['qualify.py', '--output', str(output)]), patch.object(qualify, 'preflight') as preflight:
                with self.assertRaises(FileExistsError):
                    qualify.main()
                preflight.assert_not_called()
            self.assertEqual(output.read_text(), 'existing evidence')
