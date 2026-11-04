"""Run unchanged legacy behavior in a new isolated cluster using a Go overlay.

Only the hardcoded dedicated cluster name is substituted in two source files.
Loopback, context and exact provider checks remain. Source files, retained cluster
and retained controller are never changed. This is fixture-adapted regression
evidence, not a production controller or secure-issuer qualification.
"""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import time
import uuid

ROOT = Path(__file__).resolve().parents[2]
CACHE = ROOT.parent / '.cache'
KIND = ROOT.parent / 'tools/kind.exe'
NAME = 'smp-regression-' + uuid.uuid4().hex[:8]
CONFIG = CACHE / (NAME + '.kubeconfig')
REPORT = ROOT / '.logs/sprint2-legacy-regressions.json'
BASE = ['kubectl', '--kubeconfig', str(CONFIG), '--context', 'kind-' + NAME, '--request-timeout=20s']
report = {'schema': 'sathonate.fixture-adapted-regressions/v1', 'status': 'NOT_COMPLETED', 'cluster': NAME, 'productionEnabled': False, 'awsVerified': False, 'transportOverlayOnly': True, 'tests': {}}


def run(args, data=None, timeout=240):
    p = subprocess.run([str(x) for x in args], input=data, capture_output=True, text=True, encoding='utf-8', errors='replace', timeout=timeout)
    if p.returncode:
        raise RuntimeError('Disposable legacy fixture command failed: ' + Path(str(args[0])).name + '; diagnostics withheld')
    return p.stdout


def guard():
    if run(BASE + ['get', 'node', NAME + '-control-plane', '-o', 'jsonpath={.spec.providerID}']) != 'kind://docker/' + NAME + '/' + NAME + '-control-plane':
        raise RuntimeError('Disposable regression provider mismatch')


def load(image):
    identity = run(['docker', 'image', 'inspect', '--format', '{{.Id}}', image]).strip()
    archive = CACHE / ('sprint1-public-' + identity.split(':')[1] + '.tar')
    receipt = archive.with_suffix('.sha256')
    digest = lambda p: hashlib.file_digest(p.open('rb'), 'sha256').hexdigest()
    if not archive.exists() or not receipt.exists() or digest(archive) != receipt.read_text():
        original = archive.with_suffix('.original.tar')
        run(['docker', 'image', 'save', '--platform', 'linux/amd64', '-o', original, image])
        try:
            with tarfile.open(original) as source, tarfile.open(archive, 'w') as target:
                for member in source:
                    if member.name not in ('index.json', 'oci-layout'):
                        target.addfile(member, source.extractfile(member) if member.isfile() else None)
            receipt.write_text(digest(archive))
        finally:
            original.unlink(missing_ok=True)
    with archive.open('rb') as stream:
        p = subprocess.run(['docker', 'exec', '-i', NAME + '-control-plane', 'ctr', '-n', 'k8s.io', 'images', 'import', '--platform', 'linux/amd64', '-'], stdin=stream, capture_output=True, timeout=180)
    if p.returncode:
        raise RuntimeError('Legacy fixture image transport failed')
    canonical = image if image.startswith('ghcr.io/') else 'docker.io/' + image
    rows = run(['docker', 'exec', NAME + '-control-plane', 'ctr', '-n', 'k8s.io', 'images', 'list']).splitlines()
    row = next(x.split() for x in rows if x.split() and x.split()[0] == canonical)
    run(['docker', 'exec', NAME + '-control-plane', 'ctr', '-n', 'k8s.io', 'images', 'tag', '--force', canonical, canonical.rsplit(':', 1)[0] + '@' + row[2]])
    metadata = json.loads(run(['docker', 'image', 'inspect', image]))[0]
    # Preserve original regression image-reference constants while recording the
    # imported manifest separately. Config/layer bytes are unchanged.
    for source in metadata.get('RepoDigests', []):
        digest = source.rsplit('@', 1)[1]
        run(['docker', 'exec', NAME + '-control-plane', 'ctr', '-n', 'k8s.io', 'images', 'tag', '--force', canonical, canonical + '@' + digest])
    report.setdefault('images', {})[image] = {'runtimeManifest': row[2], 'sourceIndexes': metadata.get('RepoDigests', [])}


def main():
    created = False
    tools = None
    started = time.time()
    try:
        run(['docker', 'info', '--format', '{{.ServerVersion}}'])
        with tempfile.TemporaryDirectory(prefix='smp-regression-', dir=CACHE) as directory:
            private = Path(directory)
            replacements = {}
            report['overlay'] = {}
            for relative in ('internal/deployment/kubernetes.go', 'internal/deployment/eks_local_test.go'):
                path = ROOT / relative
                source = path.read_text(encoding='utf-8')
                count = source.count('smp-qualification')
                if not count:
                    raise RuntimeError('Expected fixture transport literals missing')
                adapted = source.replace('smp-qualification', NAME)
                replacement = private / path.name
                replacement.write_text(adapted, encoding='utf-8')
                replacements[str(path)] = str(replacement)
                report['overlay'][relative] = {'sourceSHA256': hashlib.sha256(path.read_bytes()).hexdigest(), 'adaptedSHA256': hashlib.sha256(replacement.read_bytes()).hexdigest(), 'literalSubstitutions': count}
            overlay = private / 'overlay.json'
            overlay.write_text(json.dumps({'Replace': replacements}))
            run([KIND, 'create', 'cluster', '--name', NAME, '--image', 'kindest/node:v1.34.0', '--kubeconfig', CONFIG, '--wait', '180s'])
            created = True
            guard()
            if os.name == 'nt':
                run(['icacls', CONFIG, '/inheritance:r', '/grant:r', run(['whoami']).strip() + ':F'])
            source = ROOT.parent / 'dependencies/bevel-operator-fabric'
            run(BASE + ['apply', '--server-side', '-f', source / 'config/crd/bases'])
            for image in ('ghcr.io/hyperledger-bevel/bevel-operator-fabric:v1.14.0', 'sathonate/fabric-worker:eks-scoped', 'sathonate/smp-smoke:local', 'hyperledger/fabric-ca:1.5.15', 'hyperledger/fabric-peer:2.5.15', 'hyperledger/fabric-orderer:2.5.15'):
                print('Loading isolated legacy fixture image ' + image, flush=True)
                load(image)
            # Copy only public chart and tool binary; no retained Helm state.
            chart = private / 'chart'
            shutil.copytree(source / 'chart/hlf-operator', chart)
            crds = (chart / 'templates/crds').resolve()
            if private.resolve() not in crds.parents:
                raise RuntimeError('Chart cleanup escaped its isolated workspace')
            shutil.rmtree(crds)
            run(['docker','run','--rm','--network','kind','--mount','type=bind,source='+str(CONFIG)+',target=/config,readonly','--mount','type=bind,source='+str(chart)+',target=/chart,readonly','--entrypoint','/usr/local/bin/helm','sathonate/bevel-tools:dev-20261016','upgrade','--install','smp-bevel','/chart','--kubeconfig','/config','--kube-apiserver','https://'+NAME+'-control-plane:6443','--namespace','smp-system','--create-namespace','--set','image.repository=ghcr.io/hyperledger-bevel/bevel-operator-fabric','--set-string','image.tag=v1.14.0','--wait','--timeout','180s'])
            env = dict(os.environ, GOCACHE=str(CACHE / 'go-build'), GOPATH=str(CACHE / 'go'), GOPROXY='off', TMP=str(CACHE / 'resume-tmp'), TEMP=str(CACHE / 'resume-tmp'), SMP_EKS_LOCAL_E2E='1', SMP_TEST_KUBECONFIG=str(CONFIG))
            for key in ('SATHONATE_TEST_DATABASE_URL', 'SMP_SECURE_API_E2E', 'SMP_POSTGRES_TLS_E2E'):
                env.pop(key, None)
            for test in ('TestEKSWorkerLifecycleOnKind', 'TestLocalWorkerRegressionOnKind'):
                print('Running fixture-adapted ' + test, flush=True)
                p = subprocess.run(['go', 'test', '-overlay', str(overlay), './internal/deployment', '-run', '^' + test + '$', '-count=1', '-json', '-timeout', '18m'], cwd=ROOT, env=env, capture_output=True, text=True, timeout=1100)
                # Keep structured states only: never export legacy controller or
                # worker diagnostic bodies, even when a regression fails.
                states = []
                for line in p.stdout.splitlines():
                    try:
                        event = json.loads(line)
                    except ValueError:
                        continue
                    if event.get('Action') in ('pass', 'fail', 'skip'):
                        states.append({k: event[k] for k in ('Action', 'Package', 'Test', 'Elapsed') if k in event})
                report['tests'][test] = {'exitCode': p.returncode, 'states': states}
                if p.returncode:
                    raise RuntimeError('Fixture-adapted legacy regression failed; private diagnostics withheld')
            report['status'] = 'FIXTURE_ADAPTED_REGRESSIONS_PASS'
    finally:
        if created:
            guard()
            run([KIND, 'delete', 'cluster', '--name', NAME])
            CONFIG.unlink(missing_ok=True)
            report['disposableClusterRemoved'] = True
        if tools:
            run(['docker', 'rm', tools])
        report['elapsedSeconds'] = round(time.time() - started, 2)
        REPORT.write_text(json.dumps(report, indent=2) + '\n')


if __name__ == '__main__':
    main()
