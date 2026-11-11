"""Gated, single-node Sprint 7 live acceptance. Uses the existing UI fixture."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import sys

BACKEND = Path(__file__).resolve().parents[2]
CONSOLE = BACKEND.parent / 'fabric-console'
sys.path.insert(0, str(BACKEND / 'tools' / 'sprint6'))
from preflight import assess, metrics

REQUIRED_CHECKS = (
    'scheduledCaptureAndExactRestorePreparation', 'offlineColdVolumeRecovery',
    'previewedRetainedDataDeletion', 'coldPrivateArtifactsRemoved',
)
BROWSER_CHECKS = ('visualBuilderInitialSave', 'visualBuilderExpansionSave',
                  'initialBrowserProvisioningAndTransaction',
                  'reviewedExpansionMembershipChannelAnchorAndCodeUpgrade',
                  'liveMonitoringAndAuthorization', 'peerOrdererCARecoveryAndTransactions',
                  'crossTenantProjectDenied', 'responsiveDashboard')


def validate_receipt(report):
    return (report.get('status') == 'PASS'
            and report.get('disposableClusterRemoved') is True
            and report.get('fixtureDatabaseScopeRemoved') is True
            and report.get('retainedNetwork', {}).get('ready') is True
            and all(report.get('checks', {}).get(k) == 'PASS' for k in REQUIRED_CHECKS)
            and all(report.get('browser', {}).get('checks', {}).get(k) == 'PASS' for k in BROWSER_CHECKS))


def preflight():
    try:
        observed = metrics()
        return {**assess(observed), 'metrics': observed}
    except (OSError, ValueError, RuntimeError, subprocess.TimeoutExpired, AttributeError) as error:
        return {'status': 'BLOCKED_ENVIRONMENT', 'failureCategory': type(error).__name__}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', required=True, type=Path)
    parser.add_argument('--run', action='store_true', help='run the disposable fixture after preflight')
    args = parser.parse_args()
    # Claim the evidence path before work, preserving any accepted prior evidence.
    with args.output.open('x', encoding='utf-8') as output:
        receipt = preflight()
        receipt['liveAcceptance'] = 'NOT_TESTED'
        receipt['fixtureInvoked'] = False
        if receipt['status'] == 'PASS_PREFLIGHT_ONLY' and args.run:
            try:
                execute(receipt)
            except (OSError, ValueError, RuntimeError, subprocess.SubprocessError, KeyboardInterrupt) as error:
                receipt.update(status='FAIL', failureCategory=type(error).__name__)
        json.dump(receipt, output, indent=2)
        output.write('\n')
    print(receipt['status'])
    return 0 if receipt['status'] in ('PASS', 'PASS_PREFLIGHT_ONLY') else 2


def execute(receipt):
    label = 'dynamic-browser-s7-' + secrets.token_hex(8)
    evidence = CONSOLE / '.logs' / label
    evidence.mkdir(parents=True, exist_ok=False)
    cache = Path(os.environ.get('SMP_CACHE_ROOT', 'E:/Sathonate/.cache' if os.name == 'nt' else str(BACKEND.parent / '.cache'))).resolve()
    kind = Path(os.environ.get('SMP_KIND_PATH', 'E:/Sathonate/tools/kind.exe' if os.name == 'nt' else shutil.which('kind') or 'kind')).resolve()
    if not kind.is_file() or not shutil.which('kubectl'):
        raise RuntimeError('local kind and kubectl required')
    env = {k: v for k, v in os.environ.items() if not k.startswith('SMP_UI_') and k not in ('KUBECONFIG','DATABASE_URL','SATHONATE_TEST_DATABASE_URL')}
    env.update(SMP_BACKEND_ROOT=str(BACKEND), SMP_CACHE_ROOT=str(cache), SMP_KIND_PATH=str(kind),
               SMP_UI_EVIDENCE_LABEL=label, SMP_UI_DYNAMIC='1', SMP_UI_SPRINT5='1', SMP_UI_SPRINT5_WORKER='1',
               SMP_UI_SPRINT5_OPERATIONS='1', SMP_UI_SPRINT7='1', SMP_UI_CALICO='1',
               GOCACHE=str(cache / 'go-build'), GOPATH=str(cache / 'go'), GOPROXY='off',
               GOOS='windows' if os.name == 'nt' else 'linux', GOARCH='amd64')
    api = evidence / ('fabric-api.exe' if os.name == 'nt' else 'fabric-api')
    try:
        with (evidence / 'console-build.log').open('w', encoding='utf-8') as log:
            subprocess.run(['node', 'node_modules/typescript/bin/tsc', '-b'], cwd=CONSOLE, env=env,
                           stdout=log, stderr=subprocess.STDOUT, timeout=180, check=True)
            subprocess.run(['node', 'node_modules/vite/bin/vite.js', 'build'], cwd=CONSOLE, env=env,
                           stdout=log, stderr=subprocess.STDOUT, timeout=180, check=True)
        with (evidence / 'api-build.log').open('w', encoding='utf-8') as log:
            subprocess.run(['go', 'build', '-o', str(api), './cmd/fabric-api'], cwd=BACKEND, env=env,
                           stdout=log, stderr=subprocess.STDOUT, timeout=240, check=False)
        if not api.is_file():
            raise RuntimeError('current-source API build failed')
        env.update(SMP_UI_API_BINARY=str(api), SMP_UI_API_SHA256=hashlib.sha256(api.read_bytes()).hexdigest())
        receipt['beforeFixture'] = preflight()
        if receipt['beforeFixture']['status'] != 'PASS_PREFLIGHT_ONLY':
            receipt['status'] = receipt['beforeFixture']['status']
            return
        receipt['evidenceDirectory'] = str(evidence)
        receipt['fixtureInvoked'] = True
        proc = subprocess.run([sys.executable, 'tools/qualify-ui.py'], cwd=CONSOLE, env=env, check=False)
        live = json.loads((evidence / 'ui-qualification.json').read_text(encoding='utf-8'))
        receipt.update(fixtureExit=proc.returncode, fixtureStatus=live.get('status'),
                       disposableClusterRemoved=live.get('disposableClusterRemoved', False))
        if live.get('resourcePreflight', {}).get('status', '').startswith('BLOCKED'):
            receipt['status'] = live['resourcePreflight']['status']
        else:
            receipt['status'] = receipt['liveAcceptance'] = 'PASS' if proc.returncode == 0 and validate_receipt(live) else 'FAIL'
    finally:
        api.unlink(missing_ok=True)


if __name__ == '__main__':
    sys.exit(main())
