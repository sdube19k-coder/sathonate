"""Redacted reproducibility evidence. Fixed local commands only; never contacts AWS.
Log bodies, environment variables, kubeconfig and configuration files are not exported.
"""
import argparse, hashlib, json, re, subprocess
from datetime import datetime, timezone
from pathlib import Path
ROOT = Path(__file__).resolve().parents[2]
def run(args):
    p = subprocess.run(args, cwd=ROOT, capture_output=True, text=True)
    return p.stdout.strip() if p.returncode == 0 else None
def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()
def collect():
    files = sorted(p for d in ('cmd','internal','scripts','tools') for p in (ROOT/d).rglob('*')
                   if p.is_file() and (p.suffix in ('.go','.py','.ps1','.tmpl','.yaml','.yml') or p.name.startswith('Dockerfile')))
    files += [ROOT/'go.mod', ROOT/'go.sum']
    h=hashlib.sha256()
    for p in files:
        h.update(p.relative_to(ROOT).as_posix().encode()+b'\0'+bytes.fromhex(digest(p)))
    revision=run(['git','rev-parse','--verify','HEAD'])
    if not revision or not re.fullmatch('[a-f0-9]{40}',revision): revision=None
    report={'schema':'sathonate.enterprise-evidence/v1','capturedAt':datetime.now(timezone.utc).isoformat(),
            'sourceRevision':revision,'sourceClean':run(['git','status','--porcelain','--untracked-files=all'])=='',
            'sourceContentSHA256':h.hexdigest(),'sourceFileCount':len(files),
            'bevelCommit':'cd236ccc61d1d159b3dadb4625d08bb5682a8451',
            'awsQualified':False,'productionExecutionEnabled':False,'images':{},'binaries':{},'testLogs':{}}
    for name in ('fabric-worker','kubectl-hlf-scoped','fabric-acceptance.exe','fabric-eks.exe','fabric-maintenance.exe','fabric-api-eks.exe','controller-candidate'):
        p=ROOT/'bin'/name
        if p.is_file(): report['binaries'][name]=digest(p)
    for tag in ('sathonate/fabric-worker:eks-scoped','sathonate/smp-smoke:local','sathonate/controller-candidate:lab','hyperledger/fabric-ca:1.5.15','hyperledger/fabric-peer:2.5.15','hyperledger/fabric-orderer:2.5.15'):
        value=run(['docker','image','inspect','--format','{{.Id}}',tag])
        if value and re.fullmatch('sha256:[a-f0-9]{64}',value): report['images'][tag]=value
    for name in ('enterprise-baseline-tests.log','enterprise-baseline-ca.log','enterprise-baseline-kind.log',
                 'enterprise-baseline-postgres.log','enterprise-final-tests.log','enterprise-final-kind.log',
                 'enterprise-final-ca.log','enterprise-final-postgres.log','enterprise-final-security.log','enterprise-maintenance-postgres.log','milestone-tests.log','milestone-postgres.log','milestone-kind.log','milestone-vet.log','milestone-cli-build.log','passwordfree-ca-prototype.log','passwordfree-pinned-validator.log','controller-candidate-build.log','controller-regression-tests.log','controller-regression-postgres.log','controller-regression-kind.log','controller-regression-vet.log','controller-lab-safety-tests.log','controller-evidence-tests.log','controller-registry-build.log','controller-registry-kind.log','controller-registry-tests.log','controller-registry-postgres.log','controller-registry-regressions.log','controller-registry-vet.log','controller-registry-cli-build.log','controller-registry-kind-readonly-failure.log','controller-registry-kind-sqlite-failure.log','controller-registry-kind-clientconfig-failure.log','controller-registry-kind-clientmode-failure.log','controller-lifecycle-build.log','controller-lifecycle-go.log','controller-lifecycle-vet.log','controller-lifecycle-postgres.log','controller-lifecycle-safety.log','controller-lifecycle-kind.log','controller-lifecycle-cli.log','controller-lifecycle-regressions.log','controller-lifecycle-kind-schema-failure.log','controller-lifecycle-kind-channel-failure.log','controller-lifecycle-kind-clientprogram-failure.log','controller-lifecycle-kind-peerjoin-failure.log','controller-lifecycle-kind-chaincodeschema-failure.log','controller-lifecycle-evidence-tests.log','controller-lifecycle-candidate-vet.log'):
        p=ROOT/'.logs'/name
        if not p.is_file(): continue
        body=p.read_text(encoding='utf-8-sig',errors='replace')
        cases={}
        for state,test in re.findall(r'^--- (PASS|FAIL|SKIP): ([A-Za-z0-9_/]+)',body,re.M): cases[test]=state
        report['testLogs'][name]={'sha256':digest(p),'cases':cases,
                                 'packageFailures':len(re.findall(r'^FAIL\s',body,re.M)),
                                 'runtimeFailure':bool(re.search(r'^Traceback \(most recent call last\):|^RuntimeError:',body,re.M)),
                                 'note':'SKIP is not PASS; empty cases may be package-only output'}
        unittest_run=re.search(r'^Ran ([0-9]+) tests? in [0-9.]+s$',body,re.M)
        if unittest_run:
            skipped=re.search(r'^OK \(skipped=([0-9]+)\)$',body,re.M)
            report['testLogs'][name]['unittest']={'count':int(unittest_run.group(1)),'skipped':int(skipped.group(1)) if skipped else 0,'status':'PASS' if re.search(r'^OK(?: \(skipped=[0-9]+\))?$',body,re.M) else 'FAIL' if re.search(r'^FAILED',body,re.M) else 'NOT VERIFIED'}
    inspection=ROOT/'.logs/milestone-stored-cr-inspection.json'
    if inspection.is_file():
        data=json.loads(inspection.read_text())
        report['storedCRInspection']={'sha256':digest(inspection),'passwordFree':data.get('passwordFree') is True,'resourceCount':len(data.get('resources',[]))}
    permissions=ROOT/'.logs/milestone-controller-permissions.json'
    if permissions.is_file():
        report['controllerPermissionAudit']={'sha256':digest(permissions),'sharedTenantContainment':'BLOCKED'}
    health=ROOT/'.logs/milestone-retained-health.json'
    if health.is_file():
        report['retainedNetworkHealth']={'sha256':digest(health),'note':'Read-only deployment readiness is not a fresh transaction proof'}
    lab=ROOT/'.logs/controller-candidate-kind.json'
    allowed_checks={'ownNamespaceSecretRead','foreignNamespaceSecretRead','clusterWideSecretList','foreignFabricRead','clusterRoleBindingCreate','foreignNamespaceCleanup','foreignSecretCleanup','foreignSecretPreserved','unsafeCAAndForeignCertificateReconciliationRefused','noFabricWorkloadCreated','actualProbeCRsContainNoEnrollmentPasswords','controllerLogsContainNoPrivateKeyMarkers','controllerRestart','restartRetainsCAFailClosed'}
    if lab.is_file():
        data=json.loads(lab.read_text())
        report['controllerCandidateProbe']={'sha256':digest(lab),'scope':'Rejected schema fixtures; NOT an operational Fabric network','fullFabricLifecycle':'BLOCKED','status':data.get('status') if data.get('status') in ('NOT_COMPLETED','FAILED','SECURITY_PROBES_PASS_FULL_FABRIC_BLOCKED') else 'NOT VERIFIED','checks':{k:v for k,v in data.get('checks',{}).items() if k in allowed_checks and v in ('PASS','FAIL','BLOCKED','NOT VERIFIED')},'disposableClusterRemoved':data.get('disposableClusterRemoved') is True}
        candidate_image=data.get('candidateImage','')
        if re.fullmatch(r'sathonate/controller-candidate@sha256:[a-f0-9]{64}',candidate_image):report['controllerCandidateProbe']['testedImage']=candidate_image
    build=ROOT.parent/'.cache/controller-security-candidate/smp-candidate-build.json'
    if build.is_file():
        data=json.loads(build.read_text())
        folder=ROOT/'tools/controller-lab'
        inputs={p.name:digest(p) for p in folder.iterdir() if p.is_file() and (p.suffix in ('.py','.tmpl') or p.name=='Dockerfile')}
        binary=ROOT/'bin/controller-candidate'
        report['controllerCandidateBuild']={'sha256':digest(build),'currentPatchInputsMatch':inputs==data.get('patchInputs'),'currentBinaryMatches':binary.is_file() and digest(binary)==data.get('binarySHA256'),'adopted':False}
    audit=ROOT/'.logs/controller-upstream-log-audit.json'
    if audit.is_file():
        data=json.loads(audit.read_text())
        report['retainedControllerLogRisk']={'sha256':digest(audit),'upstreamPrivateKeyLoggingStatementPresent':data.get('upstreamPrivateKeyLoggingStatementPresent') is True,'privateKeyMarkerObservedInSample':data.get('privateKeyMarkerObservedInSample') is True,'rawLogsExported':False}
    retained=ROOT/'.logs/controller-retained-health.json'
    if retained.is_file():report['currentRetainedNetworkHealth']={'sha256':digest(retained)}
    runtime_images=ROOT/'.logs/controller-ca-image-evidence.json'
    if runtime_images.is_file():
        data=json.loads(runtime_images.read_text())
        refs={item.get('imageID','') for item in data.get('images',[]) if isinstance(item,dict)}
        report['actualCAProbeImages']={'sha256':digest(runtime_images),'imageIDs':sorted(ref for ref in refs if isinstance(ref,str) and re.fullmatch(r'(?:docker.io/)?(?:sathonate/controller-candidate|hyperledger/fabric-ca)@sha256:[a-f0-9]{64}|sha256:[a-f0-9]{64}',ref))}
    registry=ROOT/'.logs/controller-registry-kind.json'
    if registry.is_file():
        data=json.loads(registry.read_text())
        checks=allowed_checks|{'realCAEnrollmentFromProtectedRegistry','realRootAndTLSCAReenrollment','CARegistryPasswordsAbsentFromActualPublicResources','CARegistryPasswordsAbsentFromHelmHistory','CARuntimeConfigurationMemoryBacked','CAEnrollmentSurvivesPodRestart','CARegistryPasswordsAbsentFromSecretMetadata','CARegistryPasswordsAbsentFromControllerAndCALogs'}
        report['protectedCARegistryProbe']={'sha256':digest(registry),'scope':'CA bootstrap and enrollment only; complete Fabric lifecycle NOT VERIFIED','fullFabricLifecycle':'BLOCKED','status':data.get('status') if data.get('status') in ('NOT_COMPLETED','FAILED','SECURITY_PROBES_PASS_FULL_FABRIC_BLOCKED') else 'NOT VERIFIED','checks':{k:v for k,v in data.get('checks',{}).items() if k in checks and v in ('PASS','FAIL','BLOCKED','NOT VERIFIED')},'disposableClusterRemoved':data.get('disposableClusterRemoved') is True}
        candidate=data.get('candidateImage','')
        report['protectedCARegistryProbe']['caBootstrap']=data.get('protectedCABootstrap') if data.get('protectedCABootstrap') in ('PASS','FAIL','BLOCKED') else 'NOT VERIFIED'
        if re.fullmatch(r'sathonate/controller-candidate@sha256:[a-f0-9]{64}',candidate):report['protectedCARegistryProbe']['testedImage']=candidate
    lifecycle=ROOT/'.logs/controller-lifecycle-kind.json'
    if lifecycle.is_file():
        data=json.loads(lifecycle.read_text())
        stage_names={'ca','identities','orderers','peers','channel','chaincode','recovery','rotation','security'}
        check_names=allowed_checks|{'distinctEnrollmentCredentials','noClusterChannelAuthority','actualPublicResourcesContainNoCredentials','actualCredentialFieldsEmpty','workloadAPITokensDisabled','helmValuesAndManifestsContainNoPrivateMaterial','sampledRuntimeLogsContainNoPrivateMaterial','issuerProcessArgumentsContainNoCredentials','activeCAProcessArgumentsContainNoCredentials','issuerCannotReadSecrets','issuerCannotReadForeignFabric','controllerAndNodeRestartRecovered','missingReferenceRefusedWithoutEnrollmentFallback','missingReferenceRecovery','peerTLSCertificateReenrolled','livePeerServesRenewedCertificate','certificateUpdateQueuedDuringControllerOutage','interruptedCertificateReconciliationRecovered'}|{'committedTransactions-'+name for name in ('initial','restart','recovered','tls-renewal','rotated-restart')}|{'runtimeLogs-'+name for name in ('initial','restart','recovered','tls-renewal','rotated-restart')}
        stages={name:value for name,value in data.get('stages',{}).items() if name in stage_names and value in ('PASS','FAIL','BLOCKED')}
        checks={name:value for name,value in data.get('checks',{}).items() if name in check_names and value in ('PASS','FAIL','BLOCKED','NOT VERIFIED')}
        required_checks={'actualCredentialFieldsEmpty','actualPublicResourcesContainNoCredentials','helmValuesAndManifestsContainNoPrivateMaterial','sampledRuntimeLogsContainNoPrivateMaterial','issuerProcessArgumentsContainNoCredentials','activeCAProcessArgumentsContainNoCredentials','issuerCannotReadSecrets','issuerCannotReadForeignFabric','workloadAPITokensDisabled','controllerAndNodeRestartRecovered','missingReferenceRefusedWithoutEnrollmentFallback','missingReferenceRecovery','peerTLSCertificateReenrolled','livePeerServesRenewedCertificate','certificateUpdateQueuedDuringControllerOutage','interruptedCertificateReconciliationRecovered','foreignNamespaceSecretRead','clusterWideSecretList','foreignNamespaceCleanup','foreignSecretCleanup'}|{'committedTransactions-'+name for name in ('initial','restart','recovered','tls-renewal','rotated-restart')}|{'runtimeLogs-'+name for name in ('initial','restart','recovered','tls-renewal','rotated-restart')}
        passed=data.get('status')=='LOCAL_CANDIDATE_LIFECYCLE_PASS' and data.get('disposableClusterRemoved') is True and set(stages)==stage_names and all(value=='PASS' for value in stages.values()) and all(checks.get(name)=='PASS' for name in required_checks)
        entry={'sha256':digest(lifecycle),'scope':'Real disposable kind only; never AWS qualification','stages':stages,'checks':checks,'fullFabricLifecycle':'PASS' if passed else 'BLOCKED','disposableClusterRemoved':data.get('disposableClusterRemoved') is True,'milestone2Complete':False}
        ref=data.get('candidateImage','')
        if isinstance(ref,str) and re.fullmatch(r'sathonate/controller-candidate@sha256:[a-f0-9]{64}',ref):entry['testedImage']=ref
        entry['runtimeImageDigests']=sorted(ref for ref in data.get('runtimeImageDigests',[]) if isinstance(ref,str) and re.fullmatch(r'sha256:[a-f0-9]{64}',ref))
        report['secureControllerLifecycle']=entry
    return report
if __name__=='__main__':
    ap=argparse.ArgumentParser();ap.add_argument('--output',required=True);args=ap.parse_args()
    path=Path(args.output);path.parent.mkdir(parents=True,exist_ok=True)
    path.write_text(json.dumps(collect(),indent=2)+'\n',encoding='utf-8')
    print('Redacted local evidence written; AWS qualification remains false')
