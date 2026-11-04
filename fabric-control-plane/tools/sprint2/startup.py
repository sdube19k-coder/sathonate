"""Negative startup acceptance only; never supplies real credentials/targets."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]


def main():
    binary = ROOT / 'bin/fabric-api-sprint2.exe'
    env = dict(os.environ)
    for name in ('DATABASE_URL', 'OKTA_ISSUER', 'OKTA_CLIENT_ID', 'OKTA_AUDIENCE', 'OIDC_MEMBERSHIPS_FILE'):
        env.pop(name, None)
    cases = [
        ('ambiguous-local-token', ['-local-token-grants', 'nonexistent'], 'requires explicit'),
        ('production-local-token', ['-mode', 'production', '-local-token-grants', 'nonexistent'], 'restricted to explicit local-development'),
        ('staging-local-token', ['-mode', 'staging', '-local-token-grants', 'nonexistent'], 'restricted to explicit local-development'),
        ('production-execution', ['-mode', 'production', '-deployment-mode', 'secure-local'], 'restricted to local-development'),
        ('production-database-required', ['-mode', 'production'], 'requires PostgreSQL'),
        ('cloud-execution', ['-enable-cloud-execution'], 'not qualified'),
        ('ambiguous-local-oidc',['-local-oidc'],'requires explicit local-development'),
        ('production-local-oidc',['-mode','production','-local-oidc'],'requires explicit local-development'),
        ('mixed-authentication',['-mode','local-development','-local-oidc','-local-token-grants','nonexistent'],'requires explicit local-development'),
        ('local-oidc-cloud',['-mode','local-development','-local-oidc','-deployment-mode','eks-qualification'],'requires explicit local-development'),
        ('nonloopback-local-oidc',['-mode','local-development','-local-oidc','-addr','0.0.0.0:8080'],'must bind a loopback'),
        ('local-oidc-database-required',['-mode','local-development','-local-oidc'],'local OIDC requires PostgreSQL'),
    ]
    checks = {}
    with tempfile.TemporaryDirectory(prefix='smp-startup-') as directory:
        for name, args, expected in cases:
            p = subprocess.run([str(binary), '-data', str(Path(directory) / 'drafts.json')] + args, capture_output=True, text=True, env=env, timeout=5)
            checks[name] = 'PASS' if p.returncode != 0 and expected in p.stderr else 'FAIL'
    report = {'schema': 'sathonate.sprint2-startup-denials/v1', 'apiBinarySHA256': hashlib.sha256(binary.read_bytes()).hexdigest(), 'checks': checks, 'productionEnabled': False, 'awsEnabled': False}
    (ROOT / '.logs/sprint2-startup-denials.json').write_text(json.dumps(report, indent=2) + '\n')
    if any(x != 'PASS' for x in checks.values()):
        raise RuntimeError('Unsafe startup acceptance failed; diagnostics withheld')
    print('Fail-closed native API startup cases PASS: 12')


if __name__ == '__main__':
    main()
