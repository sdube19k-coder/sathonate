"""Disposable real Kubernetes security probe, not a Fabric transaction qualification.
Creates a fresh kind cluster only; never uses ambient credentials or contexts.
"""
import argparse,hashlib,json,os,re,subprocess,time,uuid
from pathlib import Path
from datetime import datetime,timezone
import yaml
import secrets,base64,gzip,tarfile
PIN='cd236ccc61d1d159b3dadb4625d08bb5682a8451'
NS='smp-candidate'
class Probe:
    def __init__(self,a):
        self.a=a;self.name='smp-controller-'+uuid.uuid4().hex[:8]
        self.config=Path(a.cache).resolve()/(self.name+'.kubeconfig')
        if self.config.exists():raise RuntimeError('Refusing existing kubeconfig path')
        self.base=['kubectl','--kubeconfig',str(self.config),'--context','kind-'+self.name]
        self.result={'schema':'sathonate.controller-lab/v1','capturedAt':datetime.now(timezone.utc).isoformat(),'bevelCommit':PIN,'cluster':self.name,'checks':{},'fullFabricLifecycle':'BLOCKED','awsQualified':False,'productionEnabled':False,'status':'NOT_COMPLETED'}
        self.created=False
    def run(self,args,data=None,ok=True):
        p=subprocess.run(args,input=data,capture_output=True,text=True,encoding="utf-8",errors="replace",timeout=240)
        if ok and p.returncode:
            if Path(args[0]).name.lower() in ('kind','kind.exe') or ('ctr' in args and 'import' in args):print(p.stderr[:4000],flush=True)
            raise RuntimeError('Lab command failed; diagnostics withheld: '+Path(args[0]).name)
        return p
    def kub(self,*args,**kwargs):return self.run(self.base+list(args),**kwargs)
    def apply(self,value):self.kub('apply','--server-side','--field-manager=smp-controller-lab','-f','-',data=json.dumps(value))
    def check(self,name,passed):
        self.result['checks'][name]='PASS' if passed else 'FAIL'
        if not passed:raise RuntimeError('Security probe failed: '+name)
    def guard(self):
        actual=self.kub('get','node',self.name+'-control-plane','-o','jsonpath={.spec.providerID}').stdout
        if actual!='kind://docker/'+self.name+'/'+self.name+'-control-plane':raise RuntimeError('Disposable cluster provider mismatch')
    def setup(self):
        if self.name in self.run([self.a.kind,'get','clusters']).stdout.split():raise RuntimeError('Refusing existing cluster')
        print('Creating fresh disposable candidate cluster',flush=True)
        self.run([self.a.kind,'create','cluster','--name',self.name,'--image','kindest/node:v1.34.0','--kubeconfig',str(self.config),'--wait','180s']);self.created=True
        if os.name=='nt':
            identity=self.run(['whoami']).stdout.strip()
            self.run(['icacls',str(self.config),'/inheritance:r','/grant:r',identity+':F'])
        else:os.chmod(self.config,0o600)
        self.guard()
        self.run([self.a.kind,'load','docker-image',self.a.image,'--name',self.name])
        # Import only the installed platform; Docker Desktop may retain a
        # multi-platform index with absent ARM blobs that kind --all-platforms rejects.
        archive=Path(self.a.cache).resolve()/(self.name+'-ca-image.tar')
        try:
            self.run(['docker','image','save','--platform','linux/amd64','-o',str(archive),'hyperledger/fabric-ca:1.5.15'])
            # Use the archive's legacy Docker manifest: Desktop also exports an
            # OCI index with attestation references whose blobs may be absent.
            # Config/layer bytes are unchanged; only redundant OCI index metadata
            # is excluded from this disposable local image transport.
            normalized=archive.with_suffix('.normalized.tar')
            try:
                with tarfile.open(archive) as original,tarfile.open(normalized,'w') as clean:
                    for member in original:
                        if member.name in ('index.json','oci-layout'):continue
                        clean.addfile(member,original.extractfile(member) if member.isfile() else None)
                with normalized.open('rb') as image_stream:
                    imported=subprocess.run(['docker','exec','-i',self.name+'-control-plane','ctr','-n','k8s.io','images','import','--platform','linux/amd64','-'],stdin=image_stream,capture_output=True,timeout=180)
                if imported.returncode:
                    print(imported.stderr.decode('utf-8',errors='replace')[:4000],flush=True)
                    raise RuntimeError('Local CA image stream import failed')
            finally:normalized.unlink(missing_ok=True)
        finally:
            archive.unlink(missing_ok=True)
        print('Installing pinned schemas and namespace-scoped candidate in disposable cluster',flush=True)
        self.kub('apply','--server-side','-f',str(Path(self.a.source)/'config/crd/bases'))
        for ns in (NS,'smp-foreign'):self.apply({'apiVersion':'v1','kind':'Namespace','metadata':{'name':ns,'labels':{'fabric.sathonate.io/tenant':ns,'fabric.sathonate.io/network':ns}}})
        for ns in (NS,'smp-foreign'):
            self.apply({'apiVersion':'v1','kind':'Secret','metadata':{'name':'boundary-canary','namespace':ns},'stringData':{'canary':'isolated-test-only'}})
        self.apply({'apiVersion':'v1','kind':'ServiceAccount','metadata':{'name':'candidate','namespace':NS}})
        rules=[{'apiGroups':[''],'resources':['events'],'verbs':['create','patch']},{'apiGroups':[''],'resources':['secrets','configmaps','services','persistentvolumeclaims'],'verbs':['get','list','watch','create','update','patch','delete']},
               {'apiGroups':[''],'resources':['pods'],'verbs':['get','list','watch','delete']},
               {'apiGroups':['apps'],'resources':['deployments','replicasets'],'verbs':['get','list','watch','create','update','patch','delete']},
               {'apiGroups':['coordination.k8s.io'],'resources':['leases'],'verbs':['get','list','watch','create','update','patch','delete']},
               {'apiGroups':['hlf.kungfusoftware.es'],'resources':[r+s for r in ('fabriccas','fabricpeers','fabricorderernodes','fabricchaincodes') for s in ('','/status','/finalizers')],'verbs':['get','list','watch','create','update','patch','delete']}]
        self.apply({'apiVersion':'rbac.authorization.k8s.io/v1','kind':'Role','metadata':{'name':'candidate','namespace':NS},'rules':rules})
        self.apply({'apiVersion':'rbac.authorization.k8s.io/v1','kind':'RoleBinding','metadata':{'name':'candidate','namespace':NS},'roleRef':{'apiGroup':'rbac.authorization.k8s.io','kind':'Role','name':'candidate'},'subjects':[{'kind':'ServiceAccount','name':'candidate','namespace':NS}]})
        listing=self.run(['docker','exec',self.name+'-control-plane','ctr','-n','k8s.io','images','list']).stdout
        fields=next((line.split() for line in listing.splitlines() if line.split() and line.split()[0]=='docker.io/'+self.a.image),None)
        if fields is None or not re.fullmatch('sha256:[a-f0-9]{64}',fields[2]):raise RuntimeError('Loaded candidate digest unavailable')
        image='sathonate/controller-candidate@'+fields[2]
        self.run(['docker','exec',self.name+'-control-plane','ctr','-n','k8s.io','images','tag','--force','docker.io/'+self.a.image,'docker.io/'+image])
        self.result['candidateImage']=image
        container={'name':'controller','image':image,'imagePullPolicy':'Never','env':[{'name':'SMP_NAMESPACE','value':NS},{'name':'SMP_CONFIG_INIT_IMAGE','value':image}],'resources':{'requests':{'memory':'128Mi','cpu':'100m'},'limits':{'memory':'512Mi','cpu':'2'}},'securityContext':{'allowPrivilegeEscalation':False,'runAsNonRoot':True,'capabilities':{'drop':['ALL']},'seccompProfile':{'type':'RuntimeDefault'}}}
        self.apply({'apiVersion':'apps/v1','kind':'Deployment','metadata':{'name':'candidate','namespace':NS},'spec':{'replicas':1,'selector':{'matchLabels':{'app':'candidate'}},'template':{'metadata':{'labels':{'app':'candidate'}},'spec':{'serviceAccountName':'candidate','containers':[container]}}}})
        self.kub('-n',NS,'rollout','status','deployment/candidate','--timeout=180s')
    def denied(self,name,args):
        p=self.kub('--as=system:serviceaccount:'+NS+':candidate',*args,ok=False)
        self.check(name,p.returncode!=0 and 'forbidden' in p.stderr.lower())
    def exercise(self):
        print('Testing actual Kubernetes authorization and rejected CR reconciliation',flush=True)
        p=self.kub('--as=system:serviceaccount:'+NS+':candidate','-n',NS,'get','secret','boundary-canary','-o','name')
        self.check('ownNamespaceSecretRead',p.returncode==0)
        self.denied('foreignNamespaceSecretRead',['-n','smp-foreign','get','secret','boundary-canary'])
        self.denied('clusterWideSecretList',['get','secrets','--all-namespaces'])
        self.denied('foreignFabricRead',['-n','smp-foreign','get','fabricpeers'])
        self.denied('clusterRoleBindingCreate',['create','clusterrolebinding','smp-denied','--clusterrole=cluster-admin','--serviceaccount='+NS+':candidate'])
        self.denied('foreignNamespaceCleanup',['delete','namespace','smp-foreign'])
        self.denied('foreignSecretCleanup',['-n','smp-foreign','delete','secret','boundary-canary'])
        self.check('foreignSecretPreserved',self.kub('-n','smp-foreign','get','secret','boundary-canary','-o','name').returncode==0)
        # Negative fixtures use only fields from the exact pinned structural
        # schema. These are rejected reconciliation probes, not Fabric networks.
        def fixture(schema):
            if 'default' in schema:return schema['default']
            if 'enum' in schema:return schema['enum'][0]
            kind=schema.get('type')
            if kind=='object':return {k:fixture(v) for k,v in schema.get('properties',{}).items() if k in schema.get('required',[]) or 'default' in v}
            if kind=='array':return [fixture(schema['items']) for _ in range(schema.get('minItems',0))]
            if kind=='boolean':return False
            if kind in ('integer','number'):return max(schema.get('minimum',0),1)
            if kind=='string':return 'lab' if schema.get('minLength',0)>0 else ''
            return None
        def clear_credentials(v):
            if isinstance(v,dict):
                for key,value in list(v.items()):
                    if key.lower() in ('pass','password','enrollsecret'):v[key]=''
                    else:clear_credentials(value)
            elif isinstance(v,list):
                for value in v:clear_credentials(value)
        for kind,resource,api_kind in (('ca','fabriccas','FabricCA'),('peer','fabricpeers','FabricPeer'),('ordnode','fabricorderernodes','FabricOrdererNode')):
            path=Path(self.a.source)/('config/crd/bases/hlf.kungfusoftware.es_'+resource+'.yaml')
            schema=yaml.safe_load(path.read_text())['spec']['versions'][0]['schema']['openAPIV3Schema']['properties']['spec']
            obj={'apiVersion':'hlf.kungfusoftware.es/v1alpha1','kind':api_kind,'metadata':{'name':'probe-'+kind,'namespace':NS},'spec':fixture(schema)}
            clear_credentials(obj)
            if kind=='ca':
                for key in ('ca','tlsCA'):obj['spec'][key]['registry']['identities']=[]
            else:
                for key in ('component','tls'):
                    secret=obj['spec'].setdefault('secret',fixture(schema['properties']['secret']))
                    enrollment=secret.setdefault('enrollment',{}).setdefault(key,fixture(schema['properties']['secret']['properties']['enrollment']['properties'][key]))
                    enrollment.pop('enrollsecret',None)
                    enrollment['external']={'secretName':'unavailable','secretNamespace':'smp-foreign','certificateKey':'cert.pem','privateKeyKey':'key.pem','rootCertificateKey':'root.pem'}
            clear_credentials(obj)
            self.apply(obj)
        for kind in ('fabriccas','fabricpeers','fabricorderernodes'):
            deadline=time.monotonic()+75
            while time.monotonic()<deadline:
                items=json.loads(self.kub('-n',NS,'get',kind,'-o','json').stdout)['items']
                if items and items[0].get('status',{}).get('status')=='FAILED':break
                time.sleep(2)
            else:
                logs=self.kub('-n',NS,'logs','deployment/candidate',ok=False).stdout
                self.result['diagnostics']={'resource':kind,'observedStatuses':[x.get('status',{}).get('status','ABSENT') for x in items],'forbiddenInControllerLogs':'forbidden' in logs.lower(),'logLines':logs.count('\n'),'templateWatchDenied':'fabricchaincodetemplates' in logs and 'forbidden' in logs.lower()}
                raise RuntimeError('Candidate refusal status not observed')
        self.check('unsafeCAAndForeignCertificateReconciliationRefused',True)
        self.check('noFabricWorkloadCreated',len(json.loads(self.kub('-n',NS,'get','deployment','-o','json').stdout)['items'])==1)
        actual=json.loads(self.kub('-n',NS,'get','fabriccas,fabricpeers,fabricorderernodes','-o','json').stdout)
        def has_password(v):
            if isinstance(v,dict):return any((k.lower() in ('pass','password','enrollsecret') and isinstance(x,str) and bool(x)) or has_password(x) for k,x in v.items())
            if isinstance(v,list):return any(map(has_password,v))
            return False
        self.result['storedProbeCRsHaveEnrollmentPasswords']=any(has_password(x.get('spec',{})) for x in actual['items'])
        self.check('actualProbeCRsContainNoEnrollmentPasswords',not self.result['storedProbeCRsHaveEnrollmentPasswords'])
        logs=self.kub('-n',NS,'logs','deployment/candidate').stdout
        self.check('controllerLogsContainNoPrivateKeyMarkers','PRIVATE KEY-----' not in logs)
        self.kub('-n',NS,'rollout','restart','deployment/candidate')
        self.kub('-n',NS,'rollout','status','deployment/candidate','--timeout=120s')
        self.check('controllerRestart',True)
        time.sleep(3)
        observed=json.loads(self.kub('-n',NS,'get','fabriccas','probe-ca','-o','json').stdout)
        self.check('restartRetainsCAFailClosed',observed.get('status',{}).get('status')=='FAILED')
        self.result['status']='SECURITY_PROBES_PASS_FULL_FABRIC_BLOCKED'
    def ca_bootstrap(self):
        print('Testing real protected CA bootstrap, enrollment and restart',flush=True)
        passwords=[secrets.token_hex(32),secrets.token_hex(32)]
        attrs={'hf.Registrar.Roles':'peer,orderer,admin,client','hf.Registrar.DelegateRoles':'','hf.Registrar.Attributes':'','hf.Revoker':False,'hf.IntermediateCA':False,'hf.GenCRL':False,'hf.AffiliationMgr':False}
        bundle={kind:{'maxenrollments':-1,'identities':[{'name':'registrar','pass':password,'type':'client','affiliation':'','attrs':attrs}]} for kind,password in zip(('ca','tlsCA'),passwords)}
        self.apply({'apiVersion':'v1','kind':'Secret','metadata':{'name':'ca-registry-v1','namespace':NS,'labels':{'sathonate.io/namespace':NS}},'immutable':True,'stringData':{'registry.json':json.dumps(bundle)}})
        args=['ca','create','--name','bootstrap-ca','--namespace',NS,'--image','hyperledger/fabric-ca','--version','1.5.15','--storage-class','standard','--capacity','1Gi','--enroll-id','registrar','--enroll-pw',secrets.token_hex(32),'--output']
        rendered=self.run(['docker','run','--rm','-i','--network','none','--mount','type=bind,source='+str(self.config)+',target=/private/kubeconfig,readonly','-e','KUBECONFIG=/private/kubeconfig','--entrypoint','kubectl-hlf','sathonate/fabric-worker:eks-scoped','--smp-stdin-arguments'],data=json.dumps(args)).stdout
        resource=yaml.safe_load(rendered)
        resource['metadata'].setdefault('annotations',{})['sathonate.io/ca-registry-secret']='ca-registry-v1'
        resource['spec']['env']=[]
        for kind in ('ca','tlsCA'):resource['spec'][kind]['registry']['identities']=[]
        resource['spec']['service']['type']='ClusterIP'
        self.apply(resource)
        deadline=time.monotonic()+180
        while time.monotonic()<deadline:
            response=self.kub('-n',NS,'get','deployment','bootstrap-ca','-o','json',ok=False)
            if response.returncode==0:break
            time.sleep(2)
        else:raise RuntimeError('Protected CA deployment was not reconciled')
        self.kub('-n',NS,'rollout','status','deployment/bootstrap-ca','--timeout=180s')
        ca=json.loads(self.kub('-n',NS,'get','fabriccas','bootstrap-ca','-o','json').stdout)
        root=ca.get('status',{}).get('tls_cert','')
        deadline=time.monotonic()+45
        while not root and time.monotonic()<deadline:
            time.sleep(2);ca=json.loads(self.kub('-n',NS,'get','fabriccas','bootstrap-ca','-o','json').stdout);root=ca.get('status',{}).get('tls_cert','')
        if not root:raise RuntimeError('CA public TLS evidence unavailable')
        # Enrollment credentials exist only in an immutable private Job Secret.
        client={'url':'https://registrar:'+passwords[0]+'@bootstrap-ca.'+NS+':7054','caname':'ca'}
        tls_client={'url':'https://registrar:'+passwords[1]+'@bootstrap-ca.'+NS+':7054','caname':'tlsca','enrollment':{'profile':'tls'}}
        self.apply({'apiVersion':'v1','kind':'Secret','metadata':{'name':'ca-client','namespace':NS},'immutable':True,'stringData':{'client.json':json.dumps(client),'tls-client.json':json.dumps(tls_client),'root.pem':root}})
        def enroll(name,tls=False):
            source='/private/tls-client.json' if tls else '/private/client.json'
            config='/workspace/client.json'
            # Fabric CA updates client configuration after enrollment. Copy into
            # RAM first; the projected Secret must remain read-only.
            command=('set -eu; umask 077; cat '+source+' >'+config+'; '
                     'failure() { echo "$1"; if grep -qi "permission denied" /workspace/private.log; then echo private_file_permission_denied; fi; exit 1; }; '
                     'fabric-ca-client enroll --config '+config+' --mspdir /workspace/msp --tls.certfiles /private/root.pem >/workspace/private.log 2>&1 || failure enrollment_failed; '
                     'test -s /workspace/msp/signcerts/cert.pem || failure certificate_missing; before=$(sha256sum /workspace/msp/signcerts/cert.pem); '
                     'fabric-ca-client reenroll --config '+config+' --mspdir /workspace/msp --tls.certfiles /private/root.pem >>/workspace/private.log 2>&1 || failure reenrollment_failed; '
                     'after=$(sha256sum /workspace/msp/signcerts/cert.pem); test "$before" != "$after"')
            pod={'restartPolicy':'Never','automountServiceAccountToken':False,'securityContext':{'runAsUser':65532,'runAsGroup':65532,'runAsNonRoot':True,'fsGroup':65532},
                 'volumes':[{'name':'private','secret':{'secretName':'ca-client','defaultMode':288}},{'name':'workspace','emptyDir':{'medium':'Memory','sizeLimit':'8Mi'}}],
                 'containers':[{'name':'enroll','image':'hyperledger/fabric-ca:1.5.15','imagePullPolicy':'IfNotPresent','env':[{'name':'FABRIC_CA_CLIENT_HOME','value':'/workspace'}],
                                'securityContext':{'allowPrivilegeEscalation':False,'capabilities':{'drop':['ALL']}},'command':['sh','-c',command],
                                'volumeMounts':[{'name':'private','mountPath':'/private','readOnly':True},{'name':'workspace','mountPath':'/workspace'}]}]}
            self.apply({'apiVersion':'batch/v1','kind':'Job','metadata':{'name':name,'namespace':NS},'spec':{'backoffLimit':0,'activeDeadlineSeconds':90,'template':{'spec':pod}}})
            self.kub('-n',NS,'wait','--for=condition=complete','job/'+name,'--timeout=100s')
        enroll('initial-enrollment')
        enroll('tls-enrollment',tls=True)
        self.check('realCAEnrollmentFromProtectedRegistry',True)
        self.check('realRootAndTLSCAReenrollment',True)
        # Check actual API objects and decoded Helm storage, in memory only.
        public=json.loads(self.kub('-n',NS,'get','fabriccas,configmaps,deployments,jobs','-o','json').stdout)
        payload=json.dumps(public)
        self.check('CARegistryPasswordsAbsentFromActualPublicResources',not any(p in payload for p in passwords))
        metadata=json.loads(self.kub('-n',NS,'get','secrets','-o','json').stdout)['items']
        self.check('CARegistryPasswordsAbsentFromSecretMetadata',not any(p in json.dumps(item.get('metadata',{})) for item in metadata for p in passwords))
        releases=json.loads(self.kub('-n',NS,'get','secrets','-l','owner=helm','-o','json').stdout)['items']
        if not releases:raise RuntimeError('Helm history inspection unavailable')
        for release in releases:
            raw=base64.b64decode(base64.b64decode(release['data']['release']))
            if raw[:2]==b'\x1f\x8b':raw=gzip.decompress(raw)
            if any(p.encode() in raw for p in passwords):raise RuntimeError('Registry credential found in Helm history; diagnostics withheld')
        self.check('CARegistryPasswordsAbsentFromHelmHistory',True)
        for deployment_name in ('candidate','bootstrap-ca'):
            diagnostics=self.kub('-n',NS,'logs','deployment/'+deployment_name,'--all-containers=true').stdout
            if any(p in diagnostics for p in passwords):raise RuntimeError('Registry credential found in private log inspection; diagnostics withheld')
        self.check('CARegistryPasswordsAbsentFromControllerAndCALogs',True)
        deployment=json.loads(self.kub('-n',NS,'get','deployment','bootstrap-ca','-o','json').stdout)
        volumes=deployment['spec']['template']['spec']['volumes']
        self.check('CARuntimeConfigurationMemoryBacked',any(v['name']=='smp-runtime' and v.get('emptyDir',{}).get('medium')=='Memory' for v in volumes))
        self.kub('-n',NS,'rollout','restart','deployment/bootstrap-ca')
        self.kub('-n',NS,'rollout','status','deployment/bootstrap-ca','--timeout=180s')
        enroll('after-restart-enrollment')
        self.check('CAEnrollmentSurvivesPodRestart',True)
        self.result['protectedCABootstrap']='PASS'
        self.result['fullFabricLifecycle']='BLOCKED'
    def finish(self):
        if self.created:
            self.guard()
            self.run([self.a.kind,'delete','cluster','--name',self.name])
            self.config.unlink(missing_ok=True)
            self.result['disposableClusterRemoved']=True
        self.result['finishedAt']=datetime.now(timezone.utc).isoformat()
        Path(self.a.output).write_text(json.dumps(self.result,indent=2)+'\n')

def main():
    p=argparse.ArgumentParser();p.add_argument('--kind',required=True);p.add_argument('--cache',required=True);p.add_argument('--source',required=True);p.add_argument('--image',default='sathonate/controller-candidate:lab');p.add_argument('--output',required=True);p.add_argument('--ca-bootstrap',action='store_true');a=p.parse_args()
    probe=Probe(a)
    try:
        probe.setup();probe.exercise()
        if a.ca_bootstrap:probe.ca_bootstrap()
    except Exception:
        probe.result['status']='FAILED'
        raise
    finally:probe.finish()
    print('Disposable candidate security probes passed; full password-free Fabric deployment remains BLOCKED')
if __name__=='__main__':main()
