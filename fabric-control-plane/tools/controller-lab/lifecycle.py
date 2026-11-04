"""Real disposable candidate lifecycle. No production adapter imports this lab.
Private bytes remain in API Secrets, host RAM or pod memory volumes only.
"""
import argparse,base64,gzip,hashlib,io,json,re,secrets,tarfile,time
from pathlib import Path
import yaml
from probe import Probe,NS,PIN

PARTICIPATION_SCRIPT=r'''import ssl,urllib.request,pathlib,sys
boundary='smpfixedboundary'
block=pathlib.Path('/workspace/genesis.block').read_bytes()
body=('--'+boundary+'\r\nContent-Disposition: form-data; name="config-block"; filename="genesis.block"\r\nContent-Type: application/octet-stream\r\n\r\n').encode()+block+('\r\n--'+boundary+'--\r\n').encode()
context=ssl.create_default_context(cafile='/workspace/admin-root.pem')
context.load_cert_chain('/workspace/admin-cert.pem','/workspace/admin-key.pem')
response=urllib.request.urlopen(urllib.request.Request(sys.argv[1],body,{'Content-Type':'multipart/form-data; boundary='+boundary}),context=context)
assert response.status==201
'''
IDENTITY_SCRIPT='''import pathlib,json,sys
from cryptography import x509
from cryptography.hazmat.primitives import serialization as s
p=pathlib.Path(sys.argv[1]);cert=(p/'signcerts/cert.pem').read_text()
public=x509.load_pem_x509_certificate(cert.encode()).public_key().public_bytes(s.Encoding.DER,s.PublicFormat.SubjectPublicKeyInfo)
keys=[key.read_text() for key in (p/'keystore').iterdir()]
matching=[key for key in keys if s.load_pem_private_key(key.encode(),password=None).public_key().public_bytes(s.Encoding.DER,s.PublicFormat.SubjectPublicKeyInfo)==public]
assert len(matching)==1
root=next((p/('tlscacerts' if (p/'tlscacerts').exists() else 'cacerts')).iterdir()).read_text()
print(json.dumps({'tls.crt':cert,'tls.key':matching[0],'tlsroot.crt':root}))
'''

class Lifecycle(Probe):
    def __init__(self,args):
        super().__init__(args)
        self.passwords=[];self.private_keys=[];self.cas={};self.identities={}
        self.result['schema']='sathonate.controller-lifecycle/v1'
        self.result['stages']={}
    def stage(self,name):
        print('Candidate lifecycle: '+name,flush=True)
        self.result['currentStage']=name;self.result['stages'][name]='RUNNING'
    def passed(self,name):self.result['stages'][name]='PASS'
    def apply(self,value):
        raw=json.dumps(value)
        if value.get('kind')!='Secret' and (any(p in raw for p in self.passwords) or 'PRIVATE KEY-----' in raw):raise RuntimeError('Credential-bearing public resource refused')
        response=self.kub('apply','--server-side','--field-manager=smp-controller-lab','-f','-',data=raw,ok=False)
        if response.returncode:
            # Persist only field paths and a fixed category, never API diagnostics.
            self.result['rejectedFields']=sorted(set(re.findall(r'spec\.[A-Za-z0-9_.]+',response.stderr)))
            raise RuntimeError('Candidate resource apply rejected; field paths recorded, diagnostics withheld')
    def load_image(self,image):
        image_id=self.run(['docker','image','inspect','--format','{{.Id}}',image]).stdout.strip()
        if not re.fullmatch('sha256:[a-f0-9]{64}',image_id):raise RuntimeError('Local image identity unavailable')
        cached=Path(self.a.cache)/('controller-public-image-'+image_id.split(':')[1]+'.tar')
        receipt=cached.with_suffix('.sha256')
        def digest(path):
            with path.open('rb') as stream:return hashlib.file_digest(stream,'sha256').hexdigest()
        if cached.is_file() and receipt.is_file() and digest(cached)==receipt.read_text():
            with cached.open('rb') as stream:
                p=__import__('subprocess').run(['docker','exec','-i',self.name+'-control-plane','ctr','-n','k8s.io','images','import','--platform','linux/amd64','-'],stdin=stream,capture_output=True,timeout=180)
                if p.returncode:raise RuntimeError('Verified public image cache import failed')
            return
        archive=Path(self.a.cache)/(self.name+'-image.tar');normalized=archive.with_suffix('.normalized.tar')
        try:
            self.run(['docker','image','save','--platform','linux/amd64','-o',str(archive),image])
            with tarfile.open(archive) as original,tarfile.open(normalized,'w') as target:
                for member in original:
                    if member.name not in ('index.json','oci-layout'):target.addfile(member,original.extractfile(member) if member.isfile() else None)
            with normalized.open('rb') as stream:
                p=__import__('subprocess').run(['docker','exec','-i',self.name+'-control-plane','ctr','-n','k8s.io','images','import','--platform','linux/amd64','-'],stdin=stream,capture_output=True,timeout=180)
                if p.returncode:raise RuntimeError('Fixed local image import failed')
            if self.run(['docker','image','inspect','--format','{{.Id}}',image]).stdout.strip()!=image_id:raise RuntimeError('Local image changed during transport')
            normalized.replace(cached);receipt.write_text(digest(cached))
        finally:archive.unlink(missing_ok=True);normalized.unlink(missing_ok=True)
    def setup(self):
        super().setup()
        for image in ('hyperledger/fabric-peer:2.5.15','hyperledger/fabric-orderer:2.5.15','sathonate/fabric-worker:eks-scoped','sathonate/smp-smoke:local'):self.load_image(image)
    def secret(self,name,data,owner=None,immutable=False):
        labels={'sathonate.io/namespace':NS}
        if owner:labels['sathonate.io/owner']=owner
        self.apply({'apiVersion':'v1','kind':'Secret','metadata':{'name':name,'namespace':NS,'labels':labels},'immutable':immutable,'stringData':data})
    def exec(self,container,*args,data=None):
        result=self.kub('-n',NS,'exec','-i','pod/issuer','-c',container,'--',*args,data=data,ok=False)
        if result.returncode:
            diagnostic=result.stderr+result.stdout
            categories=[marker for marker in ('CERTIFICATE_VERIFY_FAILED','Connection refused','HTTP Error 400','HTTP Error 403','HTTP Error 404','HTTP Error 500','protected_ca_operation_failed','Permission denied','timed out','SyntaxError','AssertionError','NameError','endorsement failure','context deadline exceeded','no such file or directory','access denied','MSP','could not initialize','Failed to create','bad proposal response','certificate signed by unknown authority') if marker in diagnostic]
            self.result['operationFailure']={'stage':self.result.get('currentStage'),'executable':args[0] if args[0] in ('python','peer','env','sh','kubectl-hlf') else 'fixed-tool','categories':categories}
            raise RuntimeError('Candidate operation failed; classified diagnostics recorded without raw output')
        return result.stdout
    def write(self,path,data):
        if not re.fullmatch('/workspace/[a-zA-Z0-9_./-]+',path) or '..' in path:raise RuntimeError('Private workspace path refused')
        self.exec('tools','python','-c','import os,sys;os.umask(0o077);open(sys.argv[1],"w").write(sys.stdin.read())',path,data=data)
    def hlf(self,*args):return self.exec('tools','kubectl-hlf','--smp-stdin-arguments',data=json.dumps(list(args)))
    def render(self,*args):return yaml.safe_load(self.hlf(*args,'--output'))
    def wait(self,resource,name):
        deadline=time.monotonic()+210
        while time.monotonic()<deadline:
            p=self.kub('-n',NS,'get',resource,name,'-o','json',ok=False)
            if p.returncode==0 and json.loads(p.stdout).get('status',{}).get('status')=='RUNNING':
                self.kub('-n',NS,'rollout','status','deployment/'+name,'--timeout=90s');return
            time.sleep(2)
        raise RuntimeError('Candidate resource did not become ready: '+resource+'/'+name)
    def bootstrap(self):
        self.stage('ca')
        attrs={'hf.Registrar.Roles':'peer,orderer,admin,client','hf.Registrar.DelegateRoles':'','hf.Registrar.Attributes':'','hf.Revoker':False,'hf.IntermediateCA':False,'hf.GenCRL':False,'hf.AffiliationMgr':False}
        for org in ('org1','org2'):
            passwords=[secrets.token_hex(32),secrets.token_hex(32)];self.passwords.extend(passwords)
            bundle={kind:{'maxenrollments':-1,'identities':[{'name':'registrar','pass':password,'type':'client','affiliation':'','attrs':attrs}]} for kind,password in zip(('ca','tlsCA'),passwords)}
            self.secret(org+'-registry',{'registry.json':json.dumps(bundle)},immutable=True)
            args=['ca','create','--name',org+'-ca','--namespace',NS,'--image','hyperledger/fabric-ca','--version','1.5.15','--storage-class','standard','--capacity','1Gi','--enroll-id','registrar','--enroll-pw',secrets.token_hex(32),'--output']
            rendered=self.run(['docker','run','--rm','-i','--network','none','--mount','type=bind,source='+str(self.config)+',target=/private/kubeconfig,readonly','-e','KUBECONFIG=/private/kubeconfig','--entrypoint','kubectl-hlf','sathonate/fabric-worker:eks-scoped','--smp-stdin-arguments'],data=json.dumps(args)).stdout
            v=yaml.safe_load(rendered);v['metadata'].setdefault('annotations',{})['sathonate.io/ca-registry-secret']=org+'-registry'
            v['spec']['env']=[];v['spec']['service']['type']='ClusterIP'
            for kind in ('ca','tlsCA'):v['spec'][kind]['registry']['identities']=[]
            self.apply(v);self.wait('fabriccas',org+'-ca')
            self.cas[org]={'passwords':passwords,'status':json.loads(self.kub('-n',NS,'get','fabriccas',org+'-ca','-o','json').stdout)['status']}
        self.passed('ca')
    def issuer(self):
        self.stage('identities')
        self.apply({'apiVersion':'v1','kind':'ServiceAccount','metadata':{'name':'issuer','namespace':NS}})
        self.apply({'apiVersion':'rbac.authorization.k8s.io/v1','kind':'Role','metadata':{'name':'issuer','namespace':NS},'rules':[{'apiGroups':['hlf.kungfusoftware.es'],'resources':['fabriccas','fabricpeers','fabricorderernodes'],'verbs':['get','list']}]})
        self.apply({'apiVersion':'rbac.authorization.k8s.io/v1','kind':'RoleBinding','metadata':{'name':'issuer','namespace':NS},'roleRef':{'apiGroup':'rbac.authorization.k8s.io','kind':'Role','name':'issuer'},'subjects':[{'kind':'ServiceAccount','name':'issuer','namespace':NS}]})
        token={'name':'api','projected':{'sources':[{'serviceAccountToken':{'path':'token','expirationSeconds':3600}},{'configMap':{'name':'kube-root-ca.crt','items':[{'key':'ca.crt','path':'ca.crt'}]}},{'downwardAPI':{'items':[{'path':'namespace','fieldRef':{'fieldPath':'metadata.namespace'}}]}}]}}
        common={'securityContext':{'runAsUser':10001,'runAsGroup':10001,'allowPrivilegeEscalation':False,'capabilities':{'drop':['ALL']}},'command':['sh','-c','sleep 3600'],'volumeMounts':[{'name':'workspace','mountPath':'/workspace'}]}
        tools=dict(common,name='tools',image='sathonate/fabric-worker:eks-scoped',imagePullPolicy='Never',env=[{'name':'SMP_WORKER_NAMESPACE','value':NS}]);tools['volumeMounts']=common['volumeMounts']+[{'name':'api','mountPath':'/var/run/secrets/kubernetes.io/serviceaccount','readOnly':True}]
        ca=dict(common,name='client',image='hyperledger/fabric-ca:1.5.15',imagePullPolicy='Never',env=[{'name':'FABRIC_CA_CLIENT_HOME','value':'/workspace'}])
        self.apply({'apiVersion':'v1','kind':'Pod','metadata':{'name':'issuer','namespace':NS},'spec':{'shareProcessNamespace':True,'serviceAccountName':'issuer','automountServiceAccountToken':False,'securityContext':{'fsGroup':10001},'volumes':[{'name':'workspace','emptyDir':{'medium':'Memory','sizeLimit':'64Mi'}},token],'containers':[tools,ca]}})
        self.kub('-n',NS,'wait','--for=condition=Ready','pod/issuer','--timeout=120s')
        sampler='''import json,pathlib,time
p=pathlib.Path('/workspace');samples=0;operations=0;leak=False
while not (p/'audit-stop').exists():
 try:
  secrets=json.loads((p/'audit-input.json').read_text())
  for file in pathlib.Path('/proc').glob('[0-9]*/cmdline'):
   try:raw=file.read_bytes().decode(errors='replace')
   except (OSError,PermissionError):continue
   samples+=1
   if raw.startswith('fabric-ca-client\\x00'):operations+=1
   if any(value and value in raw for value in secrets):leak=True
  (p/'audit-result.json').write_text(json.dumps({'samples':samples,'operations':operations,'credentialObserved':leak}))
 except (OSError,ValueError):pass
 time.sleep(0.02)
'''
        self.write('/workspace/audit-input.json',json.dumps(self.passwords));self.write('/workspace/proc-audit.py',sampler)
        self.exec('tools','sh','-c','nohup python /workspace/proc-audit.py </dev/null >/workspace/proc-audit.log 2>&1 &')
        for org in ('org1','org2'):
            self.write('/workspace/'+org+'-server.pem',self.cas[org]['status']['tls_cert'])
            for i,caname in enumerate(('ca','tlsca')):
                key=org+'-registrar-'+caname;config={'url':'https://registrar:'+self.cas[org]['passwords'][i]+'@'+org+'-ca.'+NS+':7054','caname':caname}
                self.write('/workspace/'+key+'.json',json.dumps(config));self.ca_command(org,key,'enroll');self.read_identity(key)
        for org in ('org1','org2'):
            for role in ('channel-admin','chaincode-admin','client'):
                self.issue(org,org+'-'+role,'ca','client' if role=='client' else 'admin')
            self.issue(org,org+'-channel-tls','tlsca','admin',tls=True)
        for node,org in [('org1-peer-1','org1'),('org2-peer-1','org2')]+[('orderer-'+str(i),'org1') for i in range(1,4)]:
            role='peer' if 'peer' in node else 'orderer'
            for suffix,caname,kind in [('sign','ca',role),('tls','tlsca',role),('aux','tlsca','client')]:self.issue(org,node+'-'+suffix,caname,kind,tls=caname=='tlsca',hosts=[node,node+'.'+NS,'localhost'])
            self.material(node)
        self.issue('org1','smp-smoke-tls','tlsca','client',tls=True,hosts=['smp-smoke.'+NS,'smp-smoke'])
        self.secret('smp-smoke-external',self.bundle('smp-smoke-tls'),immutable=True)
        self.check('distinctEnrollmentCredentials',len(self.passwords)==len(set(self.passwords)))
        self.passed('identities')
    def ca_command(self,org,key,command,registrar=None):
        msp='/workspace/'+(registrar or key)+'/msp'
        # Only paths and fixed subcommands reach argv; private configuration is in RAM.
        self.exec('client','sh','-c','umask 077; fabric-ca-client '+command+' --config /workspace/'+key+'.json --mspdir '+msp+' --tls.certfiles /workspace/'+org+'-server.pem >/workspace/ca-private.log 2>&1 || { echo protected_ca_operation_failed; exit 1; }')
    def issue(self,org,key,caname,kind,tls=False,hosts=None):
        password=secrets.token_hex(32);self.passwords.append(password)
        self.write('/workspace/audit-input.json',json.dumps(self.passwords+self.private_keys))
        reg=org+'-registrar-'+caname
        config={'url':'https://'+org+'-ca.'+NS+':7054','caname':caname,'id':{'name':key,'secret':password,'type':kind}}
        self.write('/workspace/'+key+'.json',json.dumps(config));self.ca_command(org,key,'register',reg)
        config={'url':'https://'+key+':'+password+'@'+org+'-ca.'+NS+':7054','caname':caname}
        if tls:config.update({'enrollment':{'profile':'tls'},'csr':{'hosts':hosts or ['localhost']}})
        self.write('/workspace/'+key+'.json',json.dumps(config));self.ca_command(org,key,'enroll')
        self.read_identity(key)
    def read_identity(self,key):
        raw=self.exec('tools','python','-c',IDENTITY_SCRIPT,'/workspace/'+key+'/msp')
        data=json.loads(raw);self.private_keys.append(data['tls.key']);self.identities[key]=data
    def bundle(self,key):return self.identities[key]
    def material(self,node):
        for suffix in ('sign','tls'):self.secret(node+'-'+suffix+'-external',self.bundle(node+'-'+suffix),immutable=True)
        aux=self.bundle(node+'-aux')
        if 'peer' in node:
            self.secret(node+'-tls-ops',{'tls.crt':aux['tls.crt'],'tls.key':aux['tls.key']},owner=node)
            self.secret(node+'-tlsrootcert',{'cacert.pem':aux['tlsroot.crt']},owner=node)
        else:self.secret(node+'-admin',{'tls.crt':aux['tls.crt'],'tls.key':aux['tls.key'],'cacert.crt':aux['tlsroot.crt'],'clientcacert.crt':aux['tlsroot.crt']},owner=node)
    def reference(self,name):return {'secretName':name,'secretNamespace':NS,'certificateKey':'tls.crt','privateKeyKey':'tls.key','rootCertificateKey':'tlsroot.crt'}
    def nodes(self):
        for role,nodes in [('orderers',[('orderer-'+str(i),'org1') for i in range(1,4)]),('peers',[('org1-peer-1','org1'),('org2-peer-1','org2')])]:
            self.stage(role)
            for node,org in nodes:
                command='peer' if role=='peers' else 'ordnode';image='hyperledger/fabric-peer' if role=='peers' else 'hyperledger/fabric-orderer'
                args=[command,'create','--name',node,'--namespace',NS,'--image',image,'--version','2.5.15','--storage-class','standard','--capacity','1Gi','--enroll-id','unused','--enroll-pw','unused','--mspid','Org1MSP' if org=='org1' else 'Org2MSP','--ca-name',org+'-ca.'+NS,'--ca-host',org+'-ca.'+NS,'--ca-port','7054']
                if command=='peer':args.extend(['--statedb','leveldb'])
                v=self.render(*args);spec=v['spec'];spec['imagePullPolicy' if 'imagePullPolicy' in spec else 'pullPolicy']='IfNotPresent'
                for part,suffix in [('component','sign'),('tls','tls')]:
                    spec['secret']['enrollment'][part]['enrollsecret']='';spec['secret']['enrollment'][part]['external']=self.reference(node+'-'+suffix+'-external')
                if command=='peer':
                    spec['couchdb']['password']=''
                    spec['externalEndpoint']=node+'.'+NS+':7051';spec['gossip']['externalEndpoint']=spec['externalEndpoint'];spec['gossip']['endpoint']=spec['externalEndpoint']
                service=spec['service'];service['type' if 'type' in service else 'serviceType']='ClusterIP'
                self.apply(v);self.wait('fabricpeers' if command=='peer' else 'fabricorderernodes',node)
            self.passed(role)
    def peer(self,org,*args,identity='channel-admin'):
        node=org+'-peer-1';environment=['CORE_PEER_LOCALMSPID='+('Org1MSP' if org=='org1' else 'Org2MSP'),'CORE_PEER_ADDRESS='+node+'.'+NS+':7051','CORE_PEER_TLS_ENABLED=true','CORE_PEER_TLS_ROOTCERT_FILE=/workspace/'+org+'-tlsroot.pem']
        # Use a fixed public root path instead of depending on generated filenames.
        self.write('/workspace/'+org+'-tlsroot.pem',self.cas[org]['status']['tlsca_cert'])
        environment[-1]='CORE_PEER_TLS_ROOTCERT_FILE=/workspace/'+org+'-tlsroot.pem'
        environment.append('CORE_PEER_MSPCONFIGPATH=/workspace/'+org+'-'+identity+'/msp')
        # Fabric CA enrollment creates certificates, but not the local MSP's
        # NodeOU role configuration. Match the existing proven worker layout.
        msp='/workspace/'+org+'-'+identity+'/msp'
        self.write(msp+'/cacerts/identity.pem',self.cas[org]['status']['ca_cert'])
        nodeous={'Enable':True}
        for role in ('Client','Peer','Admin','Orderer'):
            nodeous[role+'OUIdentifier']={'Certificate':'cacerts/identity.pem','OrganizationalUnitIdentifier':role.lower()}
        self.write(msp+'/config.yaml',yaml.safe_dump({'NodeOUs':nodeous}))
        return self.exec('tools','env',*environment,'peer',*args)
    def channels(self):
        self.stage('channel')
        data={'namespace':NS,'organizations':[{'msp':'Org1MSP' if org=='org1' else 'Org2MSP','root':self.cas[org]['status']['ca_cert'],'tlsRoot':self.cas[org]['status']['tlsca_cert']} for org in ('org1','org2')],'orderers':[]}
        for i in range(1,4):
            node='orderer-'+str(i);r=json.loads(self.kub('-n',NS,'get','fabricorderernodes',node,'-o','json').stdout)['status']
            data['orderers'].append({'name':node,'tls':r['tlsCert'],'sign':r['signCert']})
        block=self.kub('-n',NS,'exec','-i','deployment/candidate','--','/controller-candidate','channel-genesis',data=json.dumps(data)).stdout
        self.exec('tools','python','-c','import base64,sys,pathlib;pathlib.Path("/workspace/genesis.block").write_bytes(base64.b64decode(sys.stdin.read()))',data=block)
        for i in range(1,4):
            name='orderer-'+str(i);identity=self.bundle('org1-channel-tls')
            self.write('/workspace/admin-cert.pem',identity['tls.crt']);self.write('/workspace/admin-key.pem',identity['tls.key']);self.write('/workspace/admin-root.pem',identity['tlsroot.crt'])
            # Python's TLS/multipart implementation calls Fabric's participation
            # API directly; no cluster-scoped channel CR/controller is needed.
            self.exec('tools','python','-c',PARTICIPATION_SCRIPT,'https://'+name+'.'+NS+':7053/participation/v1/channels')

        for org in ('org1','org2'):self.peer(org,'channel','join','-b','/workspace/genesis.block')
        self.check('noClusterChannelAuthority',not json.loads(self.kub('get','fabricmainchannels','-o','json').stdout)['items'])
        self.passed('channel')
    def pack(self,files):
        stream=io.BytesIO()
        with tarfile.open(fileobj=stream,mode='w:gz') as f:
            for name,data in files.items():entry=tarfile.TarInfo(name);entry.size=len(data);entry.mode=0o600;f.addfile(entry,io.BytesIO(data))
        return stream.getvalue()
    def contract(self):
        self.stage('chaincode')
        connection={'address':'smp-smoke.'+NS+':7052','domain_name':'smp-smoke.'+NS,'dial_timeout':'10s','tls_required':True,'client_auth_required':False,'root_cert':self.cas['org1']['status']['tlsca_cert']}
        code=self.pack({'connection.json':json.dumps(connection).encode()});pkg=self.pack({'metadata.json':b'{"type":"ccaas","label":"smp_smoke_1"}','code.tar.gz':code});package_id='smp_smoke_1:'+hashlib.sha256(pkg).hexdigest()
        self.exec('tools','python','-c','import sys,base64,pathlib;pathlib.Path("/workspace/chaincode.tgz").write_bytes(base64.b64decode(sys.stdin.read()))',data=base64.b64encode(pkg).decode())
        # Derive the exact CR schema through the pinned CLI, without applying its
        # password-bearing sync command. Public skeleton comes from source schema.
        self.apply({'apiVersion':'hlf.kungfusoftware.es/v1alpha1','kind':'FabricChaincode','metadata':{'name':'smp-smoke','namespace':NS},'spec':{'image':'sathonate/smp-smoke:local','imagePullPolicy':'IfNotPresent','packageId':package_id,'chaincodeServerPort':7052,'replicas':1,'credentials':{'enrollid':'unused','enrollsecret':'','cahost':'org1-ca.'+NS,'caport':7054,'caname':'tlsca','catls':{'cacert':base64.b64encode(self.cas['org1']['status']['tls_cert'].encode()).decode()},'csr':{'hosts':['smp-smoke.'+NS]},'external':self.reference('smp-smoke-external')}}})
        self.wait('fabricchaincodes','smp-smoke')
        for org in ('org1','org2'):self.peer(org,'lifecycle','chaincode','install','/workspace/chaincode.tgz',identity='chaincode-admin')
        common=['--channelID','smpchannel','--name','smp_smoke','--version','1.0','--sequence','1','--signature-policy',"AND('Org1MSP.member','Org2MSP.member')",'-o','orderer-1.'+NS+':7050','--tls','--cafile','/workspace/org1-tlsroot.pem']
        for org in ('org1','org2'):self.peer(org,'lifecycle','chaincode','approveformyorg',*common,'--package-id',package_id,identity='chaincode-admin')
        peers=[]
        for org in ('org1','org2'):peers.extend(['--peerAddresses',org+'-peer-1.'+NS+':7051','--tlsRootCertFiles','/workspace/'+org+'-tlsroot.pem'])
        self.peer('org1','lifecycle','chaincode','commit',*common,*peers,'--waitForEvent','--waitForEventTimeout','45s',identity='chaincode-admin')
        self.passed('chaincode');self.transactions('initial')
    def transactions(self,label):
        key='smp-'+label+'-'+secrets.token_hex(4);value='verified-'+label
        common=['-C','smpchannel','-n','smp_smoke']
        self.peer('org1','chaincode','invoke',*common,'-o','orderer-1.'+NS+':7050','--tls','--cafile','/workspace/org1-tlsroot.pem','--peerAddresses','org1-peer-1.'+NS+':7051','--tlsRootCertFiles','/workspace/org1-tlsroot.pem','--peerAddresses','org2-peer-1.'+NS+':7051','--tlsRootCertFiles','/workspace/org2-tlsroot.pem','--waitForEvent','--waitForEventTimeout','45s','-c',json.dumps({'Args':['Put',key,value]}),identity='client')
        for org in ('org1','org2'):
            actual=self.peer(org,'chaincode','query',*common,'-c',json.dumps({'Args':['Get',key]}),identity='client').strip()
            if actual!=value:raise RuntimeError('Committed value did not match on both peers')
        self.check('committedTransactions-'+label,True)
        self.audit_logs('runtimeLogs-'+label)
    def no_private(self,raw):
        encoded=raw if isinstance(raw,str) else raw.decode()
        return not any(secret in encoded or base64.b64encode(secret.encode()).decode() in encoded for secret in self.passwords+self.private_keys) and 'PRIVATE KEY-----' not in encoded
    def audit_logs(self,label):
        # Inspect initial and failure/recovery logs before subsequent restarts
        # destroy those pods. Never persist raw diagnostics or private material.
        items=json.loads(self.kub('-n',NS,'get','secrets','-o','json').stdout)['items']
        for item in items:
            for value in item.get('data',{}).values():
                decoded=base64.b64decode(value).decode(errors='replace')
                if 'PRIVATE KEY-----' in decoded:self.private_keys.append(decoded)
        pods=json.loads(self.kub('-n',NS,'get','pods','-o','json').stdout)['items']
        for pod in pods:
            for container in pod['spec'].get('containers',[])+pod['spec'].get('initContainers',[]):
                logs=self.kub('-n',NS,'logs',pod['metadata']['name'],'-c',container['name'],ok=False)
                if logs.returncode:raise RuntimeError('Runtime log inspection unavailable; security check not verified')
                if not self.no_private(logs.stdout):raise RuntimeError('Credential detected in sampled workload logs; diagnostics withheld')
        self.check(label,True)
    def inspect(self):
        self.stage('security')
        private=json.loads(self.kub('-n',NS,'get','secrets','-o','json').stdout)['items']
        for item in private:
            for value in item.get('data',{}).values():
                decoded=base64.b64decode(value).decode(errors='replace')
                if 'PRIVATE KEY-----' in decoded:self.private_keys.append(decoded)
        public=self.kub('-n',NS,'get','fabriccas,fabricpeers,fabricorderernodes,fabricchaincodes,configmaps,pods,deployments','-o','json').stdout
        def no_inline(value):
            if isinstance(value,dict):return all((key.lower() not in ('password','enrollsecret','pass') or child in ('',None)) and no_inline(child) for key,child in value.items())
            if isinstance(value,list):return all(no_inline(child) for child in value)
            return True
        self.check('actualCredentialFieldsEmpty',no_inline(json.loads(public)))
        clean=self.no_private
        self.check('actualPublicResourcesContainNoCredentials',clean(public))
        releases=json.loads(self.kub('-n',NS,'get','secrets','-l','owner=helm','-o','json').stdout)['items']
        if len(releases)<7:raise RuntimeError('Complete Helm release evidence unavailable')
        for release in releases:
            raw=base64.b64decode(base64.b64decode(release['data']['release']));raw=gzip.decompress(raw)
            if not clean(raw):raise RuntimeError('Credential material detected in Helm history; diagnostics withheld')
        self.check('helmValuesAndManifestsContainNoPrivateMaterial',True)
        pods=json.loads(self.kub('-n',NS,'get','pods','-o','json').stdout)['items']
        self.result['runtimeImageDigests']=sorted({c['imageID'].split('@')[-1] for pod in pods for c in pod['status'].get('containerStatuses',[])+pod['status'].get('initContainerStatuses',[]) if re.fullmatch(r'sha256:[a-f0-9]{64}',c.get('imageID','').split('@')[-1])})
        self.check('workloadAPITokensDisabled',all(pod['spec'].get('automountServiceAccountToken') is False for pod in pods if pod['metadata']['name']!='issuer' and not pod['metadata']['name'].startswith('candidate-')))
        self.audit_logs('sampledRuntimeLogsContainNoPrivateMaterial')
        process=self.exec('tools','python','-c','import pathlib,json;print(json.dumps([p.read_bytes().decode(errors="replace") for p in pathlib.Path("/proc").glob("[0-9]*/cmdline") if p.is_file()]))')
        self.check('issuerProcessArgumentsContainNoCredentials',clean(process))
        self.write('/workspace/audit-stop','stop');audit=json.loads(self.exec('tools','cat','/workspace/audit-result.json'))
        self.check('activeCAProcessArgumentsContainNoCredentials',audit['operations']>0 and audit['samples']>0 and not audit['credentialObserved'])
        self.denied('issuerCannotReadSecrets',['--as=system:serviceaccount:'+NS+':issuer','-n',NS,'get','secret','org1-registry'])
        self.denied('issuerCannotReadForeignFabric',['--as=system:serviceaccount:'+NS+':issuer','-n','smp-foreign','get','fabriccas'])
        self.passed('security')
    def recovery(self):
        self.stage('recovery')
        self.kub('-n',NS,'rollout','restart','deployment/candidate');self.kub('-n',NS,'rollout','status','deployment/candidate','--timeout=120s')
        for node in ('orderer-1','org1-peer-1'):
            self.kub('-n',NS,'rollout','restart','deployment/'+node);self.kub('-n',NS,'rollout','status','deployment/'+node,'--timeout=180s')
        self.transactions('restart');self.check('controllerAndNodeRestartRecovered',True)
        # Remove one required external reference, require rejection, restore it.
        patch={'spec':{'secret':{'enrollment':{'tls':{'external':None}}}}}
        before=json.loads(self.kub('-n',NS,'get','deployment','org1-peer-1','-o','json').stdout)['spec']['template']
        previous_logs=self.kub('-n',NS,'logs','deployment/candidate').stdout
        self.kub('-n',NS,'patch','fabricpeers','org1-peer-1','--type=merge','-p',json.dumps(patch))
        deadline=time.monotonic()+45;rejected=False
        while time.monotonic()<deadline:
            diagnostics=self.kub('-n',NS,'logs','deployment/candidate').stdout
            recent=diagnostics[len(previous_logs):] if diagnostics.startswith(previous_logs) else ''
            rejected=any('candidate external certificate authorization or material validation failed' in line and 'org1-peer-1' in line for line in recent.splitlines())
            if rejected:break
            time.sleep(1)
        after=json.loads(self.kub('-n',NS,'get','deployment','org1-peer-1','-o','json').stdout)['spec']['template']
        self.check('missingReferenceRefusedWithoutEnrollmentFallback',before==after and rejected)
        patch['spec']['secret']['enrollment']['tls']['external']=self.reference('org1-peer-1-tls-external')
        self.kub('-n',NS,'patch','fabricpeers','org1-peer-1','--type=merge','-p',json.dumps(patch));self.wait('fabricpeers','org1-peer-1')
        self.transactions('recovered');self.check('missingReferenceRecovery',True);self.passed('recovery')
    def rotation(self):
        self.stage('rotation')
        node='org1-peer-1';key=node+'-tls';old=self.bundle(key)['tls.crt']
        # Reenroll using the existing certificate/key, without a password URL.
        self.write('/workspace/'+key+'.json',json.dumps({'url':'https://org1-ca.'+NS+':7054','caname':'tlsca','enrollment':{'profile':'tls'},'csr':{'hosts':[node,node+'.'+NS,'localhost']}}))
        self.ca_command('org1',key,'reenroll');self.read_identity(key);renewed=self.bundle(key)
        self.check('peerTLSCertificateReenrolled',old!=renewed['tls.crt'])
        self.secret(node+'-tls-renewed',renewed,immutable=True)
        # Queue a real desired-state change during a confirmed controller outage.
        # This is not a claim of power-loss safety during an in-flight Helm write.
        self.kub('-n',NS,'scale','deployment/candidate','--replicas=0')
        deadline=time.monotonic()+90
        while time.monotonic()<deadline:
            if not json.loads(self.kub('-n',NS,'get','pods','-l','app=candidate','-o','json').stdout)['items']:break
            time.sleep(1)
        else:raise RuntimeError('Disposable controller did not stop for outage test')
        patch={'spec':{'secret':{'enrollment':{'tls':{'external':self.reference(node+'-tls-renewed')}}}}}
        self.kub('-n',NS,'patch','fabricpeers',node,'--type=merge','-p',json.dumps(patch))
        self.check('certificateUpdateQueuedDuringControllerOutage',True)
        self.kub('-n',NS,'scale','deployment/candidate','--replicas=1')
        self.kub('-n',NS,'rollout','status','deployment/candidate','--timeout=120s')
        checksum=hashlib.sha256(renewed['tls.crt'].encode()).hexdigest()[:60]
        deadline=time.monotonic()+150
        while time.monotonic()<deadline:
            deployment=json.loads(self.kub('-n',NS,'get','deployment',node,'-o','json').stdout)
            if deployment['spec']['template']['metadata']['labels'].get('checksum/public-tls')==checksum:break
            time.sleep(2)
        else:raise RuntimeError('Renewed public certificate did not reconcile into deployment')
        self.kub('-n',NS,'rollout','status','deployment/'+node,'--timeout=150s')
        script='import ssl,socket,hashlib;ctx=ssl.create_default_context(cafile="/workspace/org1-tlsroot.pem");s=ctx.wrap_socket(socket.create_connection(("'+node+'.'+NS+'",7051)),server_hostname="'+node+'.'+NS+'");print(hashlib.sha256(s.getpeercert(binary_form=True)).hexdigest());s.close()'
        actual=self.exec('tools','python','-c',script).strip()
        import ssl
        expected=hashlib.sha256(ssl.PEM_cert_to_DER_cert(renewed['tls.crt'])).hexdigest()
        self.check('livePeerServesRenewedCertificate',actual==expected)
        self.transactions('tls-renewal')
        self.check('interruptedCertificateReconciliationRecovered',True)
        self.kub('-n',NS,'rollout','restart','deployment/candidate');self.kub('-n',NS,'rollout','status','deployment/candidate','--timeout=120s')
        self.kub('-n',NS,'rollout','restart','deployment/'+node);self.kub('-n',NS,'rollout','status','deployment/'+node,'--timeout=150s');self.transactions('rotated-restart')
        self.passed('rotation')
    def lifecycle(self):
        self.bootstrap();self.issuer();self.nodes();self.channels();self.contract();self.recovery();self.rotation();self.inspect()
        self.result['fullFabricLifecycle']='PASS';self.result['status']='LOCAL_CANDIDATE_LIFECYCLE_PASS'

def main():
    p=argparse.ArgumentParser();p.add_argument('--kind',required=True);p.add_argument('--cache',required=True);p.add_argument('--source',required=True);p.add_argument('--image',default='sathonate/controller-candidate:lab');p.add_argument('--output',required=True);a=p.parse_args()
    lab=Lifecycle(a)
    try:lab.setup();lab.exercise();lab.lifecycle()
    except Exception:
        lab.result['status']='FAILED';stage=lab.result.get('currentStage')
        if stage:lab.result['stages'][stage]='FAIL'
        raise
    finally:lab.finish()
    print('Real candidate Fabric lifecycle passed locally; AWS and production remain disabled')
if __name__=='__main__':main()
