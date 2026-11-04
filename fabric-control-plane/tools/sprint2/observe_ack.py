"""Independent metadata/TLS observer; never mutates cluster resources."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import ssl
import subprocess
import time

ROOT = Path(__file__).resolve().parents[2]
NS = 'smp-candidate'
NODE = 'org1-peer-1'


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--cluster', required=True)
    args = parser.parse_args()
    if not re.fullmatch(r'smp-controller-[a-f0-9]{8}', args.cluster):
        raise RuntimeError('Only an explicitly named disposable atomic cluster is allowed')
    base = ['kubectl', '--kubeconfig', str(ROOT.parent / '.cache' / (args.cluster + '.kubeconfig')), '--context', 'kind-' + args.cluster, '--request-timeout=10s']

    def kub(*words):
        p = subprocess.run(base + list(words), capture_output=True, text=True, timeout=20)
        if p.returncode:
            if 'NotFound' in p.stderr:
                return None
            raise RuntimeError('Independent atomic observation failed; diagnostics withheld')
        return p.stdout

    def get(kind, name):
        raw = kub('-n', NS, 'get', kind, name, '-o', 'json')
        return json.loads(raw) if raw else {}

    def state():
        pods = json.loads(kub('-n', NS, 'get', 'pods', '-o', 'json'))['items']
        peers = {}
        for pod in pods:
            if pod['metadata'].get('labels', {}).get('release') in ('org1-peer-1', 'org2-peer-1'):
                peers[pod['metadata']['uid']] = {c['name']: c.get('restartCount', 0) for c in pod['status'].get('containerStatuses', [])}
        versions = kub('-n', NS, 'get', 'secrets', '-l', 'owner=helm,name=' + NODE, '-o', 'custom-columns=VERSION:.metadata.labels.version', '--no-headers')
        version = max(int(x) for x in versions.split())
        ready = any(p['metadata'].get('labels', {}).get('app') == 'candidate' and p['status'].get('phase') == 'Running' and p['status'].get('containerStatuses') and all(c.get('ready') for c in p['status']['containerStatuses']) for p in pods)
        return {'peers': peers, 'helmRevision': version, 'controllerAvailable': ready}

    def fingerprint():
        code = 'import ssl,socket,hashlib;c=ssl.create_default_context(cafile="/workspace/org1-tlsroot.pem");s=c.wrap_socket(socket.create_connection(("org1-peer-1.smp-candidate",7051),timeout=5),server_hostname="org1-peer-1.smp-candidate");print(hashlib.sha256(s.getpeercert(binary_form=True)).hexdigest());s.close()'
        return kub('-n', NS, 'exec', 'issuer', '-c', 'tools', '--', 'python', '-c', code).strip()

    result = {'schema': 'sathonate.sprint2-independent-ack/v1', 'status': 'NOT VERIFIED', 'cluster': args.cluster, 'resourceMutations': False}
    try:
        if kub('get', 'node', args.cluster + '-control-plane', '-o', 'jsonpath={.spec.providerID}') != 'kind://docker/' + args.cluster + '/' + args.cluster + '-control-plane':
            raise RuntimeError('Independent observer provider mismatch')
        deadline = time.monotonic() + 3600
        before = None
        ref = None
        while time.monotonic() < deadline:
            checkpoint = get('configmap', NODE + '-material-state').get('data', {})
            if before is None:
                fault = get('configmap', 'smp-atomic-faults').get('data', {})
                if fault.get('point') == 'before-ack' and fault.get('mode') == 'hold' and checkpoint.get('phase') == 'before-ack' and checkpoint.get('desired') == fault.get('bundle'):
                    ref = fault['bundle']
                    before = state()
                    raw = kub('-n', NS, 'get', 'secret', ref, '-o', r'jsonpath={.data.tls\.cert}')
                    import base64
                    expected = hashlib.sha256(ssl.PEM_cert_to_DER_cert(base64.b64decode(raw).decode())).hexdigest()
                    result['newCertificateLiveBeforeAcknowledgement'] = fingerprint() == expected
            elif checkpoint.get('phase') == 'ready' and checkpoint.get('active') == ref:
                after = state()
                if after['controllerAvailable']:
                    result.update(controllerAvailable=True, acknowledgementCompleted=True,
                                  sameMaterialReference=get('deployment', NODE)['spec']['template']['metadata']['annotations'].get('sathonate.io/material') == ref,
                                  expectedCertificateLiveAfterRecovery=fingerprint() == expected,
                                  noRedundantHelmUpgrade=before['helmRevision'] == after['helmRevision'],
                                  noPeerPodReplacement=len(before['peers']) == 2 and set(before['peers']) == set(after['peers']),
                                  noPeerContainerRestart=before['peers'] == after['peers'],
                                  helmRevisionBefore=before['helmRevision'], helmRevisionAfter=after['helmRevision'])
                    checks = [v for k, v in result.items() if k in ('controllerAvailable', 'acknowledgementCompleted', 'sameMaterialReference', 'expectedCertificateLiveAfterRecovery', 'noRedundantHelmUpgrade', 'noPeerPodReplacement', 'noPeerContainerRestart', 'newCertificateLiveBeforeAcknowledgement')]
                    if len(checks) != 8 or not all(checks):
                        raise RuntimeError('Independent acknowledgement stability check failed')
                    result['status'] = 'PASS'
                    return
            time.sleep(2)
        raise RuntimeError('Independent acknowledgement observation deadline exceeded')
    finally:
        (ROOT / '.logs/sprint2-independent-ack.json').write_text(json.dumps(result, indent=2) + '\n')


if __name__ == '__main__':
    main()
