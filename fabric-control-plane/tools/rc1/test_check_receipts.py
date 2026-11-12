import hashlib
import json
from pathlib import Path
import tempfile
import unittest

from check_receipts import GATES, evaluate


class ReceiptTest(unittest.TestCase):
    def test_bound_artifacts_and_live_review(self):
        with tempfile.TemporaryDirectory() as root:
            directory = Path(root)
            candidate = "a" * 40
            gate = "okta"
            self.assertEqual(evaluate(directory, candidate)[gate]["status"], "BLOCKED")
            artifact = directory / "redacted.txt"
            artifact.write_bytes(b"fixture")
            receipt = {
                "gate": gate,
                "candidate": candidate,
                "environment": "controlled",
                "checks": dict.fromkeys(GATES[gate], "PASS"),
                "evidence": [{"file": artifact.name, "sha256": hashlib.sha256(b"fixture").hexdigest()}],
            }
            def status():
                (directory / f"{gate}.json").write_text(json.dumps(receipt))
                return evaluate(directory, candidate)[gate]["status"]
            self.assertEqual(status(), "PARTIAL")
            receipt["environment"] = "live"
            self.assertEqual(status(), "BLOCKED")
            receipt["reviewRef"] = "review-123"
            self.assertEqual(status(), "PASS")
            self.assertEqual(evaluate(directory, "b" * 40)[gate]["status"], "BLOCKED")
            receipt["checks"][GATES[gate][0]] = "FAIL"
            self.assertEqual(status(), "BLOCKED")
            receipt["checks"].pop(GATES[gate][0])
            self.assertEqual(status(), "PARTIAL")
            artifact.write_bytes(b"changed")
            self.assertEqual(status(), "BLOCKED")
