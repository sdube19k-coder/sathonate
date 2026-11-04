"""Explicit loopback-only real-Okta development server. No grants are inferred.

Reads public OIDC configuration from private/okta-local.json, delivers the
existing development DB credential through child environment only, and uses the PostgreSQL membership registry. No identity is automatically granted.
"""
import json
import os
from pathlib import Path
import subprocess
from urllib.parse import quote

ROOT = Path(__file__).resolve().parents[2]


def capture(args):
    p = subprocess.run(args, capture_output=True, text=True, timeout=20)
    if p.returncode:
        raise RuntimeError('Development database unavailable; diagnostics withheld')
    return p.stdout


def main():
    config = json.loads((ROOT / 'private/okta-local.json').read_text())
    if set(config) != {'OKTA_ISSUER', 'OKTA_CLIENT_ID', 'OKTA_AUDIENCE'}:
        raise RuntimeError('Use exactly the supported public Okta variable names')
    metadata = json.loads(capture(['docker', 'inspect', 'smp-postgres']))[0]
    values = dict(x.split('=', 1) for x in metadata['Config']['Env'] if '=' in x)
    password = capture(['docker', 'exec', 'smp-postgres', 'cat', '/run/secrets/postgres-password']).strip()
    port = metadata['NetworkSettings']['Ports']['5432/tcp'][0]['HostPort']
    connection = 'postgres://' + quote(values['POSTGRES_USER'], safe='') + ':' + quote(password, safe='') + '@127.0.0.1:' + port + '/fabric_smp_tests?sslmode=disable'
    env = dict(os.environ, **config, DATABASE_URL=connection)
    env.pop('OIDC_MEMBERSHIPS_FILE', None)
    command = [str(ROOT / 'bin/fabric-api-sprint2.exe'), '-mode', 'local-development', '-local-oidc', '-addr', '127.0.0.1:8080', '-deployment-mode', 'disabled']
    migration=subprocess.run([str(ROOT/'bin/fabric-api-sprint2.exe'),'-mode','local-development','-migrate'],cwd=ROOT,env=env,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
    if migration.returncode:raise RuntimeError('Explicit development schema migration failed; diagnostics withheld')
    print('Starting explicit loopback Okta qualification; production/AWS execution disabled; no membership auto-granted', flush=True)
    raise SystemExit(subprocess.run(command, cwd=ROOT, env=env).returncode)


if __name__ == '__main__':
    main()
