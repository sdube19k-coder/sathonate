"""Serial real-PostgreSQL regressions using an existing disposable test DB.

The password is read only into memory and passed through the child environment.
No environment, inspect payload, connection string or private diagnostic is emitted.
Run after the API recovery test, never concurrently with that shared database test.
"""
import json
import argparse
import os
from pathlib import Path
import subprocess
from urllib.parse import quote

ROOT = Path(__file__).resolve().parents[2]
CACHE = ROOT.parent / '.cache'


def capture(args):
    p = subprocess.run(args, capture_output=True, text=True, timeout=20)
    if p.returncode:
        raise RuntimeError('Disposable database fixture unavailable; diagnostics withheld')
    return p.stdout


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, default=ROOT / '.logs/sprint2-postgres.jsonl',
                        help='Private local evidence file; select a new name to preserve prior qualification evidence')
    args = parser.parse_args()
    metadata = json.loads(capture(['docker', 'inspect', 'smp-postgres']))[0]
    values = dict(x.split('=', 1) for x in metadata['Config']['Env'] if '=' in x)
    password = capture(['docker', 'exec', 'smp-postgres', 'cat', '/run/secrets/postgres-password']).strip()
    port = metadata['NetworkSettings']['Ports']['5432/tcp'][0]['HostPort']
    connection = 'postgres://' + quote(values['POSTGRES_USER'], safe='') + ':' + quote(password, safe='') + '@127.0.0.1:' + port + '/fabric_smp_tests?sslmode=disable'
    env = dict(os.environ, GOCACHE=str(CACHE / 'go-build'), GOPATH=str(CACHE / 'go'), GOPROXY='off', TMP=str(CACHE / 'resume-tmp'), TEMP=str(CACHE / 'resume-tmp'), SATHONATE_TEST_DATABASE_URL=connection)
    for key in ('SMP_SECURE_API_E2E', 'SMP_EKS_LOCAL_E2E', 'SMP_POSTGRES_TLS_E2E'):
        env.pop(key, None)
    output = args.output
    with output.open('w', encoding='utf-8') as stream:
        p = subprocess.run(['go', 'test', '-p', '1', './internal/persistence', './internal/operations', './internal/deployment', './internal/maintenance', './internal/httpapi', '-run', 'Postgres|OperationDeploymentRLSUnderRestrictedRole|PreparedCloudSubmissionIsAtomic', '-count=1', '-json'], cwd=ROOT, env=env, stdout=stream, stderr=subprocess.STDOUT, timeout=240)
    if password and password in output.read_text(encoding='utf-8'):
        # Do not print a diagnostic or export this file if inspection fails.
        raise RuntimeError('Private credential inspection failed; evidence withheld')
    if p.returncode:
        raise RuntimeError('PostgreSQL regression failed; see redacted test states')
    print('Serial PostgreSQL regressions PASS; database credentials withheld')


if __name__ == '__main__':
    main()
