"""Offline RC1 receipt checklist. This tool cannot authorize a release."""
import argparse
import hashlib
import json
from pathlib import Path
import re

GATES = {
    "customer-kubernetes": (
        "identity-project-binding", "target-rbac-network-isolation",
        "deploy-expand-committed-transactions", "csi-tls-recovery-cleanup",
        "metered-once-no-infrastructure-charge", "monitoring-alert-delivery",
    ),
    "okta": (
        "pkce-mfa-callback-logout", "issuer-audience-jwks-expiry",
        "read-write-scope-separation", "cross-tenant-project-denial",
    ),
    "payment-provider": (
        "hosted-checkout-and-portal", "signature-replay-ordering",
        "invoice-failure-cancellation-reconciliation", "entitlement-and-audit",
    ),
    "multi-node": (
        "capacity-preflight", "cross-worker-ledger-and-tls",
        "worker-failure-raft-pvc-recovery", "timings-and-cleanup",
    ),
    "disaster-recovery": (
        "scheduled-encrypted-capture", "postgres-restore",
        "replacement-target-single-writer", "ledger-fresh-transactions-rpo-rto",
    ),
}

def evaluate(directory, candidate):
    results = {}
    for gate, expected in GATES.items():
        receipt = directory / f"{gate}.json"
        if not receipt.is_file():
            results[gate] = {"status": "BLOCKED", "reason": "missing live receipt"}
            continue
        try:
            data = json.loads(receipt.read_text(encoding="utf-8"))
            if data["candidate"] != candidate or data["gate"] != gate:
                raise ValueError("binding")
            if data["environment"] not in {"live", "controlled"}:
                raise ValueError("environment")
            checks = data["checks"]
            if not isinstance(checks, dict) or set(checks) - set(expected):
                raise ValueError("checks")
            if any(value not in {"PASS", "FAIL", "BLOCKED"} for value in checks.values()):
                raise ValueError("result")
            evidence = data["evidence"]
            if not isinstance(evidence, list) or not evidence:
                raise ValueError("evidence")
            for item in evidence:
                path = (directory / item["file"]).resolve()
                if not path.is_relative_to(directory.resolve()) or not path.is_file():
                    raise ValueError("evidence path")
                with path.open("rb") as artifact:
                    if hashlib.file_digest(artifact, "sha256").hexdigest() != item["sha256"]:
                        raise ValueError("evidence checksum")
            if data["environment"] == "controlled":
                results[gate] = {"status": "PARTIAL", "reason": "controlled receipt only"}
            elif "FAIL" in checks.values():
                results[gate] = {"status": "BLOCKED", "reason": "live check failed"}
            elif set(checks) != set(expected) or "BLOCKED" in checks.values():
                results[gate] = {"status": "PARTIAL", "reason": "live receipt incomplete"}
            elif not isinstance(data.get("reviewRef"), str) or not re.fullmatch(r"[a-zA-Z0-9._-]{3,128}", data["reviewRef"]):
                results[gate] = {"status": "BLOCKED", "reason": "missing independent review reference"}
            else:
                results[gate] = {"status": "PASS", "reason": "receipt complete; verify review authority separately"}
        except (OSError, ValueError, TypeError, KeyError, AttributeError):
            results[gate] = {"status": "BLOCKED", "reason": "invalid or unmatched receipt"}
    return results

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--list", action="store_true")
    parser.add_argument("--directory", type=Path)
    parser.add_argument("--candidate")
    args = parser.parse_args()
    if args.list:
        print(json.dumps(GATES, indent=2))
        return 0
    if args.directory is None or not args.candidate or not re.fullmatch(r"[a-f0-9]{40}", args.candidate):
        parser.error("a receipt directory and exact 40-character candidate commit are required")
    results = evaluate(args.directory, args.candidate)
    print(json.dumps(results, indent=2))
    return int(any(result["status"] != "PASS" for result in results.values()))

if __name__ == "__main__":
    raise SystemExit(main())
