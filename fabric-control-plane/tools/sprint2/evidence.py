"""Redacted Sprint 2 evidence. Never exports log bodies or environment values."""
import hashlib
import json
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[2]
LOG = ROOT / '.logs'


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def run(args):
    p = subprocess.run(args, cwd=ROOT, capture_output=True, text=True, timeout=30)
    return p.stdout.strip() if p.returncode == 0 else None


def structured_go(name):
    path = LOG / name
    if not path.exists():
        return {'status': 'NOT VERIFIED'}
    states = {'pass': [], 'fail': [], 'skip': []}
    package_failures = []
    started=set();completed=set()
    for line in path.read_text(encoding='utf-8-sig').splitlines():
        try:
            event = json.loads(line)
        except ValueError:
            continue
        action = event.get('Action')
        if not event.get('Test') and action=='start':started.add(event.get('Package'))
        if not event.get('Test') and action in ('pass','skip'):completed.add(event.get('Package'))
        if event.get('Test') and action in states:
            states[action].append(event['Test'])
        if not event.get('Test') and action in ('fail', 'build-fail'):
            package_failures.append(event.get('Package', event.get('ImportPath')))
    status='FAIL' if states['fail'] or package_failures else 'PASS' if states['pass'] and started and started<=completed else 'NOT VERIFIED'
    return {'sha256': digest(path), 'status': status, 'incompletePackages':sorted(started-completed), 'passed': len(states['pass']), 'failed': states['fail'], 'skipped': states['skip'], 'packageFailures': package_failures}


def unittest(name):
    path = LOG / name
    if not path.exists():
        return {'status': 'NOT VERIFIED'}
    text = path.read_text(encoding='utf-8-sig')
    count = re.search(r'^Ran (\d+) tests? in ', text, re.M)
    return {'sha256': digest(path), 'status': 'PASS' if count and re.search(r'^OK\s*$', text, re.M) else 'FAIL', 'count': int(count[1]) if count else None}


def collect():
    names = subprocess.check_output(['git', 'ls-files', '--cached', '--others', '--exclude-standard', '-z'], cwd=ROOT).decode().split('\0')
    files = {}
    for name in sorted(set(names)):
        path = ROOT / name
        if name and path.is_file():
            # Git on Windows may normalize text endings during commit. Hash the
            # canonical repository text, not incidental CRLF checkout encoding.
            files[name] = hashlib.sha256(path.read_bytes().replace(b'\r\n', b'\n')).hexdigest()
    aggregate = hashlib.sha256(json.dumps(files, sort_keys=True, separators=(',', ':')).encode()).hexdigest()
    report = {'schema': 'sathonate.sprint2-evidence/v1', 'overallStatus': 'BLOCKED', 'sourceRevision': run(['git', 'rev-parse', 'HEAD']), 'sourceClean': run(['git', 'status', '--porcelain', '--untracked-files=all']) == '', 'sourceContentSHA256': aggregate, 'sourceFiles': files, 'sourceHashNormalization': 'CRLF to LF', 'productionEnabled': False, 'awsEnabled': False, 'tests': {}, 'runtime': {}, 'blockers': [
        'Actual Okta acceptance: configured issuer/client/audience and scopes are available; complete the required interactive security method, verify issuer/subject, approve the existing-scope read-only grant, then qualify callback, authenticated reads/denials and logout. Controlled OIDC is not real Okta acceptance.',
        'Retained incident remediation: owner-approved public-certificate exposure inventory, migration/rotation/restore and stale-identity rejection; no retained change authorized or performed.',
        'Production deployment qualification remains disabled: target-cluster isolation/admission, approved registry enforcement, secret/encrypted backup delivery, restricted pod compatibility, audit retention and broader Fabric/CA/Raft identity recovery need independent acceptance.'
    ]}
    for key, name in [('go', 'sprint2-go-final.jsonl'), ('candidate', 'sprint2-candidate-final.jsonl'), ('postgres', 'sprint2-postgres.jsonl')]:
        report['tests'][key] = structured_go(name)
    for key, name in [('atomicPython', 'sprint2-atomic-python.log'), ('lifecyclePython', 'sprint2-lifecycle-python.log'), ('evidencePython', 'sprint2-evidence-python.log'), ('newEvidencePython','sprint2-new-evidence-python.log')]:
        report['tests'][key] = unittest(name)
    for key, name, test in [('postgresTLS', 'sprint2-postgres-tls-final.log', 'TestProductionPostgresTLSAndRoleOnDocker'), ('dockerCA', 'sprint2-docker-ca.log', 'TestProtectedCAConfigOnDocker')]:
        path = LOG / name
        if path.exists():
            text = path.read_text(encoding='utf-8-sig')
            report['tests'][key] = {'sha256': digest(path), 'status': 'PASS' if re.search(r'^--- PASS: ' + test + r' ', text, re.M) and not re.search(r'^FAIL\s*$', text, re.M) else 'FAIL'}
    vet = LOG / 'sprint2-vet-final.log'
    report['tests']['vet'] = {'status': 'PASS' if vet.exists() and not vet.read_text(encoding='utf-8-sig').strip() else 'NOT VERIFIED', 'sha256': digest(vet) if vet.exists() else None}
    for key, name in [('calicoFabric', 'sprint2-qualification.json'), ('api', 'sprint2-api-kind.json'), ('atomic', 'sprint2-atomic-kind.json'), ('independentAck','sprint2-independent-ack.json'), ('legacyFixtureAdapted', 'sprint2-legacy-regressions.json')]:
        path = LOG / name
        if path.exists():
            report['runtime'][key] = {'sha256': digest(path), 'evidence': json.loads(path.read_text(encoding='utf-8-sig'))}
        else:
            report['runtime'][key] = {'status': 'NOT VERIFIED'}
    report['runtime']['oidcQualificationScope'] = 'Controlled HTTPS OIDC/JWKS fixture; not real Okta login acceptance.'
    startup=LOG/'sprint2-startup-denials.json'
    if startup.exists():report['startupDenials']={'sha256':digest(startup),'evidence':json.loads(startup.read_text())}
    for key, name in [('realOkta', 'sprint2-real-okta-status.json'), ('approvedDevelopmentScope', 'sprint2-real-okta-scope.json')]:
        path = LOG / name
        if path.exists():
            report[key] = {'sha256': digest(path), 'evidence': json.loads(path.read_text())}
    console = ROOT.parent / 'fabric-console'
    report['console'] = {
        'sourceRevision': run(['git', '-C', str(console), 'rev-parse', 'HEAD']),
        'sourceClean': run(['git', '-C', str(console), 'status', '--porcelain']) == '',
        'evidence': {}
    }
    for name in ('sprint2-unit.log', 'sprint2-build.log', 'sprint2-format.log'):
        path = console / '.logs' / name
        if path.exists():
            report['console']['evidence'][name] = {'sha256': digest(path)}
    build = LOG / 'sprint2-build-inputs.json'
    if build.exists():
        inputs = json.loads(build.read_text())
        matches = all((ROOT / name).exists() and digest(ROOT / name) == expected for name, expected in inputs['implementationFiles'].items())
        report['implementationMatchesRuntimeInputs'] = matches
        report['buildInputsSHA256'] = digest(build)
        report['workerBinarySHA256'] = inputs['workerBinarySHA256']
        report['controllerBinarySHA256'] = inputs['controllerBinarySHA256']
    return report


if __name__ == '__main__':
    output = LOG / 'sprint2-evidence.json'
    output.write_text(json.dumps(collect(), indent=2) + '\n', encoding='utf-8')
    print('Redacted Sprint 2 evidence written; overall release remains BLOCKED')
