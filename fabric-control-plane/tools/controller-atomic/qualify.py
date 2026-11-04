"""Disposable real-cluster qualification; never selects an existing context.

This is a test harness, not an application deployment runner. No result is PASS
until the inherited Fabric lifecycle and all six actual failure cases finish.
"""
import argparse
import base64
import gzip
import hashlib
import json
import ssl
import sys
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent / 'controller-lab'))
from lifecycle import Lifecycle
from probe import NS

ANNOTATION = 'sathonate.io/material'
NODE = 'org1-peer-1'
# Default kubelet crash backoff reaches 300 seconds. Two deliberately repeated
# exits may require two such waits; allow startup/observation time as well.
CRASH_WAIT_SECONDS = 660
RECOVERY_WAIT_SECONDS = 420
TRANSPORT_PROBE = '''import json,socket,ssl
ready=True;untrusted=False
for node,org,port in [('org1-peer-1','org1',7051),('org2-peer-1','org2',7051),('orderer-1','org1',7050),('orderer-2','org1',7050),('orderer-3','org1',7050)]:
 try:
  context=ssl.create_default_context(cafile='/workspace/'+org+'-tlsroot.pem')
  context.set_alpn_protocols(['h2'])
  with socket.create_connection((node+'.smp-candidate',port),timeout=3) as raw:
   with context.wrap_socket(raw,server_hostname=node+'.smp-candidate'):pass
 except ssl.SSLCertVerificationError:ready=False;untrusted=True
 except OSError:ready=False
print(json.dumps({'ready':ready,'untrusted':untrusted}))
'''


class AtomicQualification(Lifecycle):
    def __init__(self, args):
        super().__init__(args)
        self.result.update(schema='sathonate.atomic-qualification/v1',
                           atomicQualification='NOT_VERIFIED', failureCases={})

    def exec(self, container, *args, data=None):
        response = self.kub('-n', NS, 'exec', '-i', 'pod/issuer', '-c', container, '--', *args, data=data, ok=False)
        if response.returncode:
            # Fixed diagnostic categories only, never raw peer/CA output.
            diagnostic = (response.stdout + response.stderr).lower()
            markers = ('connection refused', 'connection error', 'context deadline exceeded',
                       'timed out', 'endorsement', 'certificate', 'registration failed',
                       'container exited', 'chaincode', 'not found', 'unavailable',
                       'could not', 'failed', 'access denied', 'permission denied', 'msp')
            self.result['operationFailure'] = {'stage': self.result.get('currentStage'),
                'categories': [marker for marker in markers if marker in diagnostic],
                'exitCode': response.returncode}
            raise RuntimeError('Atomic Fabric operation failed; fixed categories recorded without raw output')
        return response.stdout

    def transport_ready(self):
        response = self.kub('-n', NS, 'exec', 'pod/issuer', '-c', 'tools', '--',
                            'python', '-c', TRANSPORT_PROBE, ok=False)
        if response.returncode:
            raise RuntimeError('Fabric transport probe failed; diagnostics withheld')
        try:
            result = json.loads(response.stdout)
        except (ValueError, TypeError):
            raise RuntimeError('Fabric transport probe returned invalid evidence') from None
        if not isinstance(result, dict) or set(result) != {'ready', 'untrusted'}:
            raise RuntimeError('Fabric transport probe returned invalid evidence')
        if result['untrusted'] is not False:
            raise RuntimeError('Fabric TLS trust validation failed')
        return result['ready'] is True

    def transactions(self, label):
        # An operations-port readiness probe can precede service endpoint/TLS
        # availability after restart. Verify real peer/orderer TLS listeners
        # twice before sending the one actual transaction; never retry a write.
        consecutive = 0
        def ready():
            nonlocal consecutive
            consecutive = consecutive + 1 if self.transport_ready() else 0
            return consecutive >= 2
        self.until(ready, 'trusted Fabric TLS endpoints', 90)
        super().transactions(label)

    def setup(self):
        if self.a.image != 'sathonate/controller-candidate:atomic':
            raise RuntimeError('Separate atomic image required')
        super().setup()
        self.guard()
        self.kub('-n', NS, 'set', 'env', 'deployment/candidate', 'SMP_ATOMIC_LAB_FAULTS=1')
        self.kub('-n', NS, 'rollout', 'status', 'deployment/candidate', '--timeout=120s')

    def get(self, resource, name):
        p = self.kub('-n', NS, 'get', resource, name, '-o', 'json', ok=False)
        if p.returncode:
            if 'NotFound' in p.stderr:
                return None
            raise RuntimeError('Atomic observation failed; diagnostics withheld')
        return json.loads(p.stdout)

    def until(self, condition, description, seconds=210):
        deadline = time.monotonic() + seconds
        while time.monotonic() < deadline:
            value = condition()
            if value:
                return value
            time.sleep(1)
        raise RuntimeError('Atomic boundary not observed: ' + description)

    def checkpoint(self):
        return (self.get('configmap', NODE + '-material-state') or {}).get('data', {})

    def ref(self):
        return self.get('deployment', NODE)['spec']['template']['metadata']['annotations'][ANNOTATION]

    def renew(self, index):
        key = NODE + '-tls'
        self.write('/workspace/' + key + '.json', json.dumps({
            'url': 'https://org1-ca.' + NS + ':7054', 'caname': 'tlsca',
            'enrollment': {'profile': 'tls'}, 'csr': {'hosts': [NODE, NODE + '.' + NS, 'localhost']}}))
        self.ca_command('org1', key, 'reenroll')
        self.read_identity(key)
        self.write('/workspace/audit-input.json', json.dumps(self.passwords + self.private_keys))
        material = self.bundle(key)
        self.secret(NODE + '-tls-atomic-' + str(index), material, immutable=True)
        uid = self.get('fabricpeers', NODE)['metadata']['uid']
        public = {'namespace': NS, 'node': NODE, 'uid': uid,
                  'sign.cert': self.bundle(NODE + '-sign')['tls.crt'],
                  'sign.root': self.bundle(NODE + '-sign')['tlsroot.crt'],
                  'tls.cert': material['tls.crt'], 'tls.root': material['tlsroot.crt'],
                  'ops.cert': self.bundle(NODE + '-aux')['tls.crt']}
        digest = hashlib.sha256(json.dumps(public, sort_keys=True, separators=(',', ':')).encode()).hexdigest()[:24]
        return NODE + '-mat-' + digest, hashlib.sha256(ssl.PEM_cert_to_DER_cert(material['tls.crt'])).hexdigest()

    def request(self, index):
        patch = {'spec': {'secret': {'enrollment': {'tls': {
            'external': self.reference(NODE + '-tls-atomic-' + str(index))}}}}}
        self.kub('-n', NS, 'patch', 'fabricpeers', NODE, '--type=merge', '-p', json.dumps(patch))

    def fault(self, ref, point, mode):
        self.apply({'apiVersion': 'v1', 'kind': 'ConfigMap',
                    'metadata': {'name': 'smp-atomic-faults', 'namespace': NS},
                    'data': {'node': NODE, 'bundle': ref, 'point': point, 'mode': mode}})

    def disarm(self):
        self.kub('-n', NS, 'delete', 'configmap', 'smp-atomic-faults', '--ignore-not-found')

    def admission(self, ref, kind, enabled):
        # Cluster-admin test fixtures exist only in this freshly guarded cluster.
        self.guard()
        name = 'smp-atomic-deny-' + kind
        if not enabled:
            for resource in ('validatingadmissionpolicybinding', 'validatingadmissionpolicy'):
                self.kub('delete', resource, name, '--ignore-not-found')
            return
        if kind == 'secret':
            group, version, resource, operations = '', 'v1', 'secrets', ['CREATE']
            expression = "object.metadata.name != '" + ref + "'"
        elif kind == 'rollout':
            group, version, resource, operations = 'apps', 'v1', 'deployments', ['UPDATE']
            expression = ("object.metadata.name != '" + NODE + "' || !has(object.spec.template.metadata.annotations) || "
                          "!('" + ANNOTATION + "' in object.spec.template.metadata.annotations) || "
                          "object.spec.template.metadata.annotations['" + ANNOTATION + "'] != '" + ref + "'")
        else:
            raise RuntimeError('Unsupported fixed admission fixture')
        self.apply({'apiVersion': 'admissionregistration.k8s.io/v1', 'kind': 'ValidatingAdmissionPolicy',
                    'metadata': {'name': name}, 'spec': {'failurePolicy': 'Fail',
                    'matchConstraints': {'resourceRules': [{'apiGroups': [group], 'apiVersions': [version],
                        'operations': operations, 'resources': [resource]}]},
                    'validations': [{'expression': expression, 'message': 'Disposable atomic failure injection'}]}})
        self.apply({'apiVersion': 'admissionregistration.k8s.io/v1', 'kind': 'ValidatingAdmissionPolicyBinding',
                    'metadata': {'name': name}, 'spec': {'policyName': name, 'validationActions': ['Deny'],
                    'matchResources': {'namespaceSelector': {'matchLabels': {'fabric.sathonate.io/tenant': NS}}}}})
        # Admission policy propagation is asynchronous. A server-side dry run
        # proves denial is active before changing the real desired reference.
        def denial_active():
            if kind == 'secret':
                obj = {'apiVersion': 'v1', 'kind': 'Secret',
                       'metadata': {'name': ref, 'namespace': NS}}
                response = self.kub('create', '--dry-run=server', '-f', '-', data=json.dumps(obj), ok=False)
            else:
                patch = {'spec': {'template': {'metadata': {'annotations': {ANNOTATION: ref}}}}}
                response = self.kub('-n', NS, 'patch', 'deployment', NODE, '--dry-run=server',
                                    '--type=merge', '-p', json.dumps(patch), ok=False)
            return response.returncode != 0 and 'Disposable atomic failure injection' in response.stderr
        self.until(denial_active, 'admission policy active', 60)

    def helm(self):
        items = json.loads(self.kub('-n', NS, 'get', 'secrets', '-l', 'owner=helm,name=' + NODE, '-o', 'json').stdout)['items']
        releases = []
        for item in items:
            raw = gzip.decompress(base64.b64decode(base64.b64decode(item['data']['release'])))
            if not self.no_private(raw):
                raise RuntimeError('Private material detected in Helm; diagnostics withheld')
            releases.append(json.loads(raw))
        return max(releases, key=lambda release: release['version'])

    def fingerprint(self):
        script = ('import ssl,socket,hashlib;ctx=ssl.create_default_context(cafile="/workspace/org1-tlsroot.pem");'
                  's=ctx.wrap_socket(socket.create_connection(("' + NODE + '.' + NS + '",7051)),'
                  'server_hostname="' + NODE + '.' + NS + '");'
                  'print(hashlib.sha256(s.getpeercert(binary_form=True)).hexdigest());s.close()')
        return self.exec('tools', 'python', '-c', script).strip()

    def restarts(self):
        pods = json.loads(self.kub('-n', NS, 'get', 'pods', '-l', 'app=candidate', '-o', 'json').stdout)['items']
        return sum(c.get('restartCount', 0) for p in pods for c in p.get('status', {}).get('containerStatuses', []))

    def peer_pods(self):
        deployment = self.get('deployment', NODE)
        selector = ','.join(k + '=' + v for k, v in sorted(deployment['spec']['selector']['matchLabels'].items()))
        pods = json.loads(self.kub('-n', NS, 'get', 'pods', '-l', selector, '-o', 'json').stdout)['items']
        return sorted(p['metadata']['uid'] for p in pods)

    def crashed(self, minimum):
        pods = json.loads(self.kub('-n', NS, 'get', 'pods', '-l', 'app=candidate', '-o', 'json').stdout)['items']
        return any(c.get('restartCount', 0) >= minimum
                   and c.get('lastState', {}).get('terminated', {}).get('exitCode') == 73
                   for p in pods for c in p.get('status', {}).get('containerStatuses', []))

    def verify_recovered(self, ref, expected, label):
        self.until(lambda: self.ref() == ref and self.checkpoint().get('phase') == 'ready'
                   and self.checkpoint().get('active') == ref, 'ready acknowledgement', RECOVERY_WAIT_SECONDS)
        self.kub('-n', NS, 'rollout', 'status', 'deployment/' + NODE, '--timeout=120s')
        if self.fingerprint() != expected:
            raise RuntimeError('Live certificate mismatch after recovery')
        self.transactions(label)
        secret = self.get('secret', ref)
        if not secret or secret.get('immutable') is not True or len(secret.get('data', {})) != 8:
            raise RuntimeError('Incomplete or mutable recovered bundle')
        if self.helm()['info']['status'] != 'deployed':
            raise RuntimeError('Helm recovery not complete')
        return secret['metadata']['uid']

    def failure_case(self, index, point, admission=None, restart_twice=False):
        print('Atomic qualification: ' + point, flush=True)
        record = {'status': 'RUNNING', 'fault': 'admission-denial' if admission else 'process-exit-73'}
        self.result['failureCases'][point] = record
        old_ref, old_cert = self.ref(), self.fingerprint()
        ref, expected = self.renew(index)
        if ref == old_ref:
            raise RuntimeError('Renewal did not produce a new material generation')
        if admission:
            self.admission(ref, admission, True)
        else:
            self.fault(ref, point, 'hold')
        self.request(index)
        if admission == 'secret':
            self.until(lambda: self.checkpoint().get('phase') == 'secret-refused', 'Secret API denial')
            if self.get('secret', ref) is not None or self.checkpoint().get('desired') == ref:
                raise RuntimeError('Rejected Secret was published')
        elif admission == 'rollout':
            self.until(lambda: self.helm()['info']['status'] == 'failed'
                       and self.helm().get('config', {}).get('smpMaterialSecret') == ref, 'rollout API denial')
        else:
            self.until(lambda: self.checkpoint().get('phase') == point
                       and self.checkpoint().get('attempt') == ref.rsplit('-mat-', 1)[1], 'checkpoint ' + point)
        if point != 'before-ack':
            if self.ref() != old_ref or self.fingerprint() != old_cert:
                raise RuntimeError('Old valid material changed before permitted rollout')
            self.transactions('atomic-old-' + str(index))
            record['previousNetworkOperational'] = True
        else:
            if self.fingerprint() != expected:
                raise RuntimeError('New certificate not live before acknowledgement')
            self.transactions('atomic-before-ack')
            record['newNetworkOperationalBeforeAcknowledgement'] = True
        before_secret = self.get('secret', ref)
        if point == 'after-secret' and self.checkpoint().get('desired') == ref:
            raise RuntimeError('Public desired reference published before validated checkpoint')
        if point == 'during-helm' and self.helm()['info']['status'] != 'pending-upgrade':
            raise RuntimeError('Actual pending Helm boundary not observed')
        version = self.helm()['version'] if point == 'before-ack' else None
        pod_uids = self.peer_pods() if point == 'before-ack' else None
        if admission:
            self.admission(ref, admission, False)
        else:
            count = self.restarts()
            self.fault(ref, point, 'crash')
            self.until(lambda: self.crashed(count + (2 if restart_twice else 1)), 'controller exit 73 and restart', CRASH_WAIT_SECONDS)
            # Logs from the previous crashed process are inspected in memory.
            logs = self.kub('-n', NS, 'logs', 'deployment/candidate', '--previous', ok=False)
            if logs.returncode or not self.no_private(logs.stdout):
                raise RuntimeError('Previous controller log inspection failed')
            self.disarm()
        # Explicit harmless CR annotation wakes reconciliation after injected API failures.
        self.kub('-n', NS, 'annotate', 'fabricpeers', NODE, 'sathonate.io/atomic-retry=' + str(index), '--overwrite')
        uid = self.verify_recovered(ref, expected, 'atomic-recovered-' + str(index))
        if before_secret and uid != before_secret['metadata']['uid']:
            raise RuntimeError('Retry replaced immutable bundle')
        if version is not None and self.helm()['version'] != version:
            raise RuntimeError('Acknowledgement recovery caused another Helm upgrade')
        if pod_uids is not None and (not pod_uids or self.peer_pods() != pod_uids):
            raise RuntimeError('Acknowledgement recovery restarted peer workloads')
        if not self.get('secret', old_ref):
            raise RuntimeError('Previous rollback material was deleted')
        record.update(status='PASS', recovered=True, immutableBundleReused=True,
                      previousBundleRetained=True, liveCertificateSHA256=expected,
                      committedAndQueriedOnBothPeers=True)

    def lifecycle(self):
        super().lifecycle()
        self.result['baseline'] = {'stages': dict(self.result['stages']), 'checks': dict(self.result['checks'])}
        self.result['status'] = 'NOT_COMPLETED'
        # The baseline inspection stops its sampler. Start a fresh one so the
        # additional real CA operations are also sampled, rather than reusing
        # an earlier successful process-argument report.
        self.exec('tools', 'python', '-c',
                  'from pathlib import Path;Path("/workspace/audit-stop").unlink(missing_ok=True);'
                  'Path("/workspace/audit-result.json").unlink(missing_ok=True)')
        self.exec('tools', 'sh', '-c', 'nohup python /workspace/proc-audit.py </dev/null >/workspace/proc-audit.log 2>&1 &')
        cases = [('before-secret', 'secret', False), ('after-secret', None, False),
                 ('during-helm', None, False), ('before-rollout', 'rollout', False),
                 ('prepared', None, True), ('before-ack', None, False)]
        for index, (point, admission, twice) in enumerate(cases, 1):
            self.failure_case(index, point, admission, twice)
        self.inspect()
        self.result.update(status='LOCAL_ATOMIC_QUALIFICATION_PASS', atomicQualification='PASS')


def main():
    parser = argparse.ArgumentParser()
    for option in ('kind', 'cache', 'source', 'output'):
        parser.add_argument('--' + option, required=True)
    parser.add_argument('--image', default='sathonate/controller-candidate:atomic')
    lab = AtomicQualification(parser.parse_args())
    try:
        lab.setup()
        lab.exercise()
        lab.lifecycle()
    except Exception:
        lab.result.update(status='FAILED', atomicQualification='FAIL')
        stage = lab.result.get('currentStage')
        if stage and lab.result['stages'].get(stage) == 'RUNNING':
            lab.result['stages'][stage] = 'FAIL'
        for case in lab.result['failureCases'].values():
            if case.get('status') == 'RUNNING':
                case['status'] = 'FAIL'
        raise
    finally:
        lab.finish()


if __name__ == '__main__':
    main()
