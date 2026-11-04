"""Allowlisted atomic evidence; no raw logs, configuration or Secret values."""
import argparse
import hashlib
import importlib.util
import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('baseline_evidence', ROOT / 'tools/engineering/evidence.py')
baseline = importlib.util.module_from_spec(spec)
spec.loader.exec_module(baseline)
POINTS = {'before-secret', 'after-secret', 'during-helm', 'before-rollout', 'prepared', 'before-ack'}
STAGES = {'ca', 'identities', 'orderers', 'peers', 'channel', 'chaincode', 'recovery', 'rotation', 'security'}


def summarize(data):
    cases = {}
    for point in sorted(POINTS):
        entry = data.get('failureCases', {}).get(point, {})
        case = {'status': entry.get('status') if entry.get('status') in ('PASS', 'FAIL', 'RUNNING') else 'NOT VERIFIED'}
        for field in ('recovered', 'immutableBundleReused', 'previousBundleRetained',
                      'committedAndQueriedOnBothPeers', 'previousNetworkOperational',
                      'newNetworkOperationalBeforeAcknowledgement'):
            case[field] = entry.get(field) is True
        digest = entry.get('liveCertificateSHA256', '')
        if isinstance(digest, str) and re.fullmatch('[a-f0-9]{64}', digest):
            case['liveCertificateSHA256'] = digest
        cases[point] = case
    source = data.get('baseline', {})
    stages = source.get('stages', {})
    checks = source.get('checks', {})
    baseline_passed = (set(stages) == STAGES and len(checks) == 42
                       and all(v == 'PASS' for v in stages.values())
                       and all(v == 'PASS' for v in checks.values()))
    required = ('recovered', 'immutableBundleReused', 'previousBundleRetained', 'committedAndQueriedOnBothPeers')
    passed = (data.get('status') == 'LOCAL_ATOMIC_QUALIFICATION_PASS'
              and data.get('atomicQualification') == 'PASS'
              and data.get('disposableClusterRemoved') is True and baseline_passed
              and all(case['status'] == 'PASS' and all(case[k] for k in required)
                      and (case['previousNetworkOperational'] or case['newNetworkOperationalBeforeAcknowledgement'])
                      for case in cases.values()))
    result = {'qualification': 'PASS' if passed else 'FAIL' if data.get('atomicQualification') == 'FAIL' else 'NOT VERIFIED', 'failureCases': cases,
              'baseline9Stages42Checks': 'PASS' if baseline_passed else 'NOT VERIFIED',
              'disposableClusterRemoved': data.get('disposableClusterRemoved') is True,
              'milestone2Complete': False, 'awsQualified': False, 'productionEnabled': False}
    image = data.get('candidateImage', '')
    if isinstance(image, str) and re.fullmatch(r'sathonate/controller-candidate@sha256:[a-f0-9]{64}', image):
        result['testedImage'] = image
    return result


def collect():
    report = baseline.collect()
    report['schema'] = 'sathonate.atomic-evidence/v1'
    report['atomicTestLogs'] = {}
    for name in ('go-test', 'go-vet', 'postgres', 'docker-ca', 'unit', 'baseline-python',
                 'evidence-tests', 'build', 'cli-build', 'kind-regressions'):
        path = ROOT / '.logs' / ('controller-atomic-' + name + '.log')
        if not path.is_file():
            continue
        body = path.read_text(encoding='utf-8-sig', errors='replace')
        states = re.findall(r'^--- (PASS|FAIL|SKIP): Test[A-Za-z0-9_]+', body, re.M)
        entry = {'sha256': baseline.digest(path),
                 'goCases': {state: states.count(state) for state in ('PASS', 'FAIL', 'SKIP')},
                 'packageFailures': len(re.findall(r'^FAIL\s', body, re.M)),
                 'runtimeFailure': bool(re.search(r'^Traceback|^RuntimeError:', body, re.M))}
        count = re.search(r'^Ran ([0-9]+) tests? in [0-9.]+s$', body, re.M)
        if count:
            entry['python'] = {'count': int(count.group(1)), 'passed': bool(re.search(r'^OK$', body, re.M))}
        report['atomicTestLogs'][name] = entry
    evidence = ROOT / '.logs/controller-atomic-kind.json'
    report['atomicPeerUpdate'] = summarize(json.loads(evidence.read_text())) if evidence.is_file() else summarize({})
    if evidence.is_file():
        report['atomicPeerUpdate']['evidenceSHA256'] = baseline.digest(evidence)
    folder = ROOT.parent / '.cache/controller-atomic-candidate'
    build = folder / 'smp-candidate-build.json'
    if build.is_file():
        data = json.loads(build.read_text())
        inputs = {str(p.relative_to(ROOT / 'tools')): baseline.digest(p)
                  for d in ('controller-atomic', 'controller-lab')
                  for p in (ROOT / 'tools' / d).iterdir()
                  if p.is_file() and (p.suffix in ('.py', '.tmpl') or p.name == 'Dockerfile')}
        binary = ROOT / 'bin/controller-atomic'
        report['atomicBuild'] = {'patchInputsMatch': inputs == data.get('patchInputs'),
                                 'binaryMatches': binary.is_file() and baseline.digest(binary) == data.get('binarySHA256'),
                                 'manifestSHA256': baseline.digest(build), 'adopted': False}
        if binary.is_file():
            report['atomicBuild']['binarySHA256'] = baseline.digest(binary)
    return report


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--output', required=True)
    args = parser.parse_args()
    Path(args.output).write_text(json.dumps(collect(), indent=2) + '\n')
    print('Redacted atomic evidence exported; AWS and production remain disabled')
