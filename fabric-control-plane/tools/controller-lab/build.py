"""Build/test an isolated fail-closed controller candidate from the exact Bevel pin.
Never edits the reference checkout or installs anything in Kubernetes.
"""
import argparse,hashlib,json,os,subprocess,tarfile,shutil
from pathlib import Path
PIN='cd236ccc61d1d159b3dadb4625d08bb5682a8451'
HERE=Path(__file__).resolve().parent

def replace(path,old,new):
    text=path.read_text(encoding='utf-8')
    if text.count(old)!=1:raise RuntimeError('Pinned source drift: '+path.name)
    path.write_text(text.replace(old,new),encoding='utf-8')

def prepare(source,build):
    if source==build or source in build.parents or build in source.parents:
        raise RuntimeError('Separate candidate build directory required')
    marker=build/'.smp-controller-lab'
    if build.exists() and any(build.iterdir()) and not marker.exists():
        raise RuntimeError('Refusing nonempty build directory without candidate marker')
    build.mkdir(parents=True,exist_ok=True);marker.write_text(PIN)
    archive=build/'source.tar'
    subprocess.run(['git','-C',str(source),'archive',PIN,'--format=tar','--output',str(archive)],check=True)
    with tarfile.open(archive) as f:f.extractall(build,filter='data')
    helper=build/'controllers/smpsecurity';helper.mkdir(exist_ok=True)
    for filename in ('security.go','security_test.go','registry.go','registry_test.go','material.go','material_test.go','channel.go'):
        (helper/filename).write_text((HERE/(filename+'.tmpl')).read_text(),encoding='utf-8')
    (build/'main.go').write_text((HERE/'main.go.tmpl').read_text(),encoding='utf-8')
    for package,kind,var in [('peer','FabricPeer','peer'),('ordnode','FabricOrdererNode','node')]:
        path=build/f'controllers/{package}/{package}_controller.go'
        replace(path,'import (','import (\n "github.com/kfsoftware/hlf-operator/controllers/smpsecurity"')
        method='ValidatePeer' if package=='peer' else 'ValidateOrdererNode'
        marker=f'func (v *ConfigValidator) {method}({var} *hlfv1alpha1.{kind}) error {{'
        guard=(f'if {var}.Spec.Secret==nil {{return errors.New("candidate external identity required")}}\n' if package=='ordnode' else '')
        guard+=f'if err:=smpsecurity.Require({var}.Namespace,{var}.Spec.Secret.Enrollment.Component,{var}.Spec.Secret.Enrollment.TLS);err!=nil{{return err}}\n'
        if package=='peer':guard+='if peer.Spec.StateDb!=hlfv1alpha1.StateDBLevelDB{return errors.New("candidate requires LevelDB profile")}\n'
        replace(path,marker,marker+'\n'+guard)
        marker=f'func (v *ConfigValidator) validateFabricCAConfig({var} *hlfv1alpha1.{kind}) error {{'
        replace(path,marker,marker+f'\n if {var}.Spec.Secret.Enrollment.Component.External!=nil{{return smpsecurity.Require({var}.Namespace,{var}.Spec.Secret.Enrollment.Component,{var}.Spec.Secret.Enrollment.TLS)}}\n')
        replace(path,'\tspec := conf.Spec\n\ttlsParams := conf.Spec.Secret.Enrollment.TLS', '''
 if conf.Spec.Secret.Enrollment.Component.External==nil || conf.Spec.Secret.Enrollment.TLS.External==nil{return nil,errors.New("candidate external identity required")}
 for _,ref:=range []*hlfv1alpha1.ExternalCertificate{conf.Spec.Secret.Enrollment.Component.External,conf.Spec.Secret.Enrollment.TLS.External}{
  if _,_,_,err:=smpsecurity.Read(context.Background(),client,namespace,ref,"");err!=nil{return nil,err}
 }
 spec := conf.Spec
 tlsParams := conf.Spec.Secret.Enrollment.TLS''')
        if package=='peer':
            replace(path,'\tif refreshCerts {\n\t\ttlsOpsCert, tlsOpsKey, _, err = CreateTLSOPSCryptoMaterial(', '''
 if tlsParams.External!=nil {
  if smpsecurity.Auxiliary(context.Background(),client,namespace,chartName,false)!=nil{return nil,errors.New("candidate peer operations identity invalid; enrollment fallback refused")}
  tlsOpsCert,tlsOpsKey,_,err=getExistingTLSOPSCrypto(client,chartName,namespace)
  if err!=nil{return nil,errors.New("candidate pre-enrolled peer operations TLS Secret required; enrollment fallback refused")}
 } else if refreshCerts {
  tlsOpsCert, tlsOpsKey, _, err = CreateTLSOPSCryptoMaterial(''')
        else:
            replace(path,'\tif refreshCerts {\n\t\tadminCert, adminKey, adminRootCert, adminClientRootCert, err = getExistingTLSAdminCrypto(', '''
 if tlsParams.External!=nil {
  if smpsecurity.Auxiliary(context.Background(),client,namespace,chartName,true)!=nil{return nil,errors.New("candidate orderer admin identity invalid; enrollment fallback refused")}
  adminCert,adminKey,adminRootCert,adminClientRootCert,err=getExistingTLSAdminCrypto(client,chartName,namespace)
  if err!=nil{return nil,errors.New("candidate pre-enrolled orderer admin TLS Secret required; enrollment fallback refused")}
 } else if refreshCerts {
  adminCert, adminKey, adminRootCert, adminClientRootCert, err = getExistingTLSAdminCrypto(''')
            replace(path,'log.Infof("Successfully created crypto material signEncodedPK=%s tlsEncodedPK=%s", signEncodedPK, tlsEncodedPK)','log.Info("Validated external crypto material")')
        body=(HERE/'validator_test.go.tmpl').read_text().replace('PACKAGE',package).replace('KIND',kind).replace('METHOD',method).replace('INITSECRET','resource.Spec.Secret=&hlfv1alpha1.Secret{}' if package=='ordnode' else '')
        body=body.replace('AUXCALL','_,err:=GetConfig(resource,client,"node","smp-candidate",nil,false,false)' if package=='peer' else '_,err:=getConfig(resource,client,"node","smp-candidate",false)')
        if package=='peer':body+='''
func TestSMPSecurityUnsupportedStateDatabaseRejected(t *testing.T){
 t.Setenv("SMP_NAMESPACE","smp-candidate");_,_,ref:=smpFixture(t)
 resource:=&hlfv1alpha1.FabricPeer{};resource.Namespace="smp-candidate"
 resource.Spec.Secret.Enrollment.Component.External=ref;resource.Spec.Secret.Enrollment.TLS.External=ref
 resource.Spec.StateDb=hlfv1alpha1.StateDBCouchDB
 err:=(&ConfigValidator{}).ValidatePeer(resource)
 if err==nil||!strings.Contains(err.Error(),"LevelDB"){t.Fatal("unsupported database profile accepted")}
}
'''
        (build/f'controllers/{package}/smp_candidate_test.go').write_text(body,encoding='utf-8')
    # Preserve supported chart Secret names, but never serialize keys into Helm.
    for package,kind,variable in [('peer','FabricPeer','c'),('ordnode','FabricOrdererNode','fabricOrdChart')]:
        path=build/f'controllers/{package}/{package}_controller.go'
        replace(path,f'\treturn &{variable}, nil',f'\tif err:=smpsecurity.ProtectChart(context.Background(),client,conf,"{kind}",&{variable});err!=nil{{return nil,err}}\n\treturn &{variable}, nil')
    secret_templates={
      'hlf-peer':['secret--couchdb.yaml','secret--peer-idkey.yaml','secret--peer-tls.yaml','secret--peer-ops-tls.yaml','secret--peerorg-tlsrootcert.yaml'],
      'hlf-ordnode':['secret--orderer0-idkey.yaml','secret--orderer0-tls.yaml','secret--orderer0-admin.yaml'],
      'hlf-ca':['secret--tls-cryptomaterial.yaml','secret--msp-cryptomaterial.yaml','secret--msp-tls-cryptomaterial.yaml'],
    }
    for chart,files in secret_templates.items():
        for filename in files:
            (build/'charts'/chart/'templates'/filename).write_text('{{/* Candidate: managed Kubernetes Secret, no private material in Helm. */}}\n')
    # Peers need no Kubernetes API token or chart-created service account/RBAC.
    for filename in ('serviceaccount.yaml','clusterrole.yaml','clusterrolebinding.yaml'):
        (build/'charts/hlf-peer/templates'/filename).write_text('{{/* Disabled in namespace-scoped candidate. */}}\n')
    replace(build/'charts/hlf-peer/templates/deployment.yaml','      serviceAccountName: {{ template "hlf-peer.fullname" . }}','      automountServiceAccountToken: false')
    replace(build/'charts/hlf-ordnode/templates/deployment.yaml','    spec:\n      hostAliases:','    spec:\n      automountServiceAccountToken: false\n      hostAliases:')
    for chart,label in [('hlf-peer','        checksum/public-tls: {{ .Values.tls.cert | sha256sum | trunc 60 }}\n        checksum/public-ops: {{ .Values.opsTLS.cert | sha256sum | trunc 60 }}'),('hlf-ordnode','        checksum/public-tls: {{ .Values.tls.cert | sha256sum | trunc 60 }}\n        checksum/public-admin: {{ .Values.admin.cert | sha256sum | trunc 60 }}'),('hlf-ca','        checksum/public-tls: {{ .Values.msp.tlsCertFile | sha256sum | trunc 60 }}')]:
        replace(build/'charts'/chart/'templates/deployment.yaml','    metadata:\n      labels:','    metadata:\n      labels:\n'+label)
    path=build/'controllers/ca/ca_controller.go'
    replace(path,'import (','import (\n "github.com/kfsoftware/hlf-operator/controllers/smpsecurity"')
    replace(path,'func (v *ConfigValidator) ValidateCA(ca *hlfv1alpha1.FabricCA) error {','''func (v *ConfigValidator) ValidateCA(ca *hlfv1alpha1.FabricCA) error {
 if smpsecurity.CAReference(ca)!=nil{return errors.New("candidate CA requires protected registry Secret delivery and empty inline registries")}
 return v.legacyValidateCA(ca)
}
func (v *ConfigValidator) legacyValidateCA(ca *hlfv1alpha1.FabricCA) error {''')
    replace(path,'return ctrl.Result{}, errors.Wrapf(err, "configuration validation failed for CA %s", hlf.Name)','''if hlf.Status.Status!=hlfv1alpha1.FailedStatus || hlf.Status.Message!=err.Error(){
  hlf.Status.Status=hlfv1alpha1.FailedStatus;hlf.Status.Message=err.Error();if hlf.Status.Conditions==nil{hlf.Status.Conditions=status.Conditions{}}
  if updateErr:=r.Status().Update(ctx,hlf);updateErr!=nil{return ctrl.Result{},errors.New("candidate blocked CA status could not be persisted")}
 }
 return ctrl.Result{},nil''')
    replace(path,'\tspec := conf.Spec\n\ttlsCert, tlsKey, err := getExistingTLSCrypto','''
 registrySecret,registryErr:=smpsecurity.RegistryReference(context.Background(),client,conf)
 if registryErr!=nil{return nil,registryErr}
 initImage:=os.Getenv("SMP_CONFIG_INIT_IMAGE")
 if !regexp.MustCompile(`^sathonate/controller-candidate@sha256:[a-f0-9]{64}$`).MatchString(initImage){return nil,errors.New("candidate configuration init image must be explicitly digest-pinned")}
 spec := conf.Spec
 tlsCert, tlsKey, err := getExistingTLSCrypto''')
    replace(path,'"reflect"','"reflect"\n "regexp"')
    replace(path,'\tk8sIP, err := utils.GetPublicIPKubernetes(clientSet)\n\tif err != nil {\n\t\treturn nil, err\n\t}', '\tk8sIP := releaseName+"."+ns // candidate exposes cluster DNS only; no node discovery permission')
    replace(path,'var c = FabricCAChart{','var c = FabricCAChart{\n RegistrySecret:registrySecret,\n ConfigInitImage:initImage,')
    replace(build/'controllers/ca/types.go','type FabricCAChart struct {','type FabricCAChart struct {\n RegistrySecret string `json:"smpRegistrySecret"`\n ConfigInitImage string `json:"smpConfigInitImage"`')
    deployment=build/'charts/hlf-ca/templates/deployment.yaml'
    # The candidate supports SQLite only: never run two CA servers on one DB.
    replace(deployment,'  strategy:\n    rollingUpdate:\n      maxUnavailable: 1','  strategy:\n    type: Recreate')
    replace(deployment,'      volumes:','      volumes:\n        - name: smp-registry\n          secret:\n            secretName: {{ required "protected registry Secret required" .Values.smpRegistrySecret }}\n            defaultMode: 0440\n        - name: smp-runtime\n          emptyDir:\n            medium: Memory\n            sizeLimit: 4Mi')
    replace(deployment,'      containers:','''      initContainers:
        - name: protected-ca-config
          image: {{ required "digest-pinned init image required" .Values.smpConfigInitImage | quote }}
          imagePullPolicy: IfNotPresent
          command: ["/controller-candidate", "ca-config"]
          securityContext:
            runAsUser: 65532
            runAsGroup: 65532
            runAsNonRoot: true
            allowPrivilegeEscalation: false
            capabilities:
              drop: ["ALL"]
          volumeMounts:
            - name: smp-registry
              mountPath: /smp/registry
              readOnly: true
            - name: ca-config
              mountPath: /smp/public-ca
              readOnly: true
            - name: ca-config-tls
              mountPath: /smp/public-tls
              readOnly: true
            - name: smp-runtime
              mountPath: /smp/runtime
      containers:''')
    replace(deployment,'    spec:\n      volumes:','    spec:\n      automountServiceAccountToken: false\n      securityContext:\n        fsGroup: 65532\n      volumes:')
    replace(deployment,'              cp /var/hyperledger/ca_config/ca.yaml $FABRIC_CA_HOME/fabric-ca-server-config.yaml','              umask 077\n              mkdir -p /var/hyperledger/smp-ca')
    replace(deployment,'              cp /var/hyperledger/ca_config_tls/fabric-ca-server-config.yaml $FABRIC_CA_HOME/fabric-ca-server-config-tls.yaml','              # Protected configuration is memory-backed; never copy it to the ledger PVC.')
    replace(deployment,'              fabric-ca-server start\n','              fabric-ca-server start --config /smp/runtime/ca.yaml\n')
    replace(deployment,'        - name: ca\n          image:', '        - name: ca\n          securityContext:\n            runAsUser: 65532\n            runAsGroup: 65532\n            runAsNonRoot: true\n            allowPrivilegeEscalation: false\n            capabilities:\n              drop: ["ALL"]\n          image:')
    # Fabric CA creates its default MSP directory relative to its config file.
    # Keep that directory in the same memory volume, never on the data PVC.
    replace(deployment,'          volumeMounts:\n            - name: data','          volumeMounts:\n            - name: smp-runtime\n              mountPath: /smp/runtime\n            - name: data')
    replace(path,'\treturn &c, nil','\tif err:=smpsecurity.ProtectChart(context.Background(),client,conf,"FabricCA",&c);err!=nil{return nil,err}\n\treturn &c, nil')
    (build/'controllers/ca/smp_candidate_test.go').write_text((HERE/'ca_test.go.tmpl').read_text(),encoding='utf-8')
    path=build/'controllers/chaincode/deploy/chaincode_deploy_controller.go'
    replace(path,'import (','import (\n "github.com/kfsoftware/hlf-operator/controllers/smpsecurity"')
    replace(path,'\tpodSpec := corev1.PodSpec{','\tnoToken:=false\n\tpodSpec := corev1.PodSpec{\n AutomountServiceAccountToken:&noToken,')
    replace(path,'\t\tWatches(\n\t\t\t&hlfv1alpha1.FabricChaincodeTemplate{},\n\t\t\tr.enqueueRequestForOwningResource(),\n\t\t).\n','')
    replace(path,'\tif fabricChaincode.Spec.Template != nil {','\tif fabricChaincode.Spec.Template != nil {return ctrl.Result{},errors.New("candidate chaincode templates are disabled")}\n\tif false && fabricChaincode.Spec.Template != nil {')

    replace(path,'\treturn fmt.Sprintf("%s-certs", fabricChaincode.Name)','''
 if fabricChaincode.Spec.Credentials!=nil && fabricChaincode.Spec.Credentials.External!=nil{return fabricChaincode.Spec.Credentials.External.SecretName}
 return fmt.Sprintf("%s-certs", fabricChaincode.Name)''')
    replace(path,'\tsecretName := r.getSecretName(m)','''
 // An external certificate Secret belongs to its issuer, never this finalizer.
 if m.Spec.Credentials!=nil && m.Spec.Credentials.External!=nil{return nil}
 secretName := r.getSecretName(m)''')
    marker='func (r FabricChaincodeDeployReconciler) getCryptoMaterial(ctx context.Context, labels map[string]string, ns string, fabricChaincode *hlfv1alpha1.FabricChaincode) (*SecretChaincodeData, error) {'
    replace(path,marker,marker+'''
 credentials:=fabricChaincode.Spec.Credentials
 if credentials==nil || credentials.Enrollsecret!="" || credentials.External==nil{return nil,errors.New("candidate pre-enrolled chaincode TLS reference required")}
 ref:=credentials.External
 if ref.CertificateKey!="tls.crt" || ref.PrivateKeyKey!="tls.key" || ref.RootCertificateKey!="tlsroot.crt"{return nil,errors.New("candidate chaincode TLS Secret key mapping invalid")}
 client,err:=kubernetes.NewForConfig(r.Config);if err!=nil{return nil,errors.New("candidate Kubernetes configuration unavailable")}
 cert,key,root,err:=smpsecurity.Read(ctx,client,ns,ref,fabricChaincode.Name+"."+ns)
 if err!=nil{return nil,err}
 return &SecretChaincodeData{Enabled:true,Updated:false,Certificate:cert,PrivateKey:key,RootCert:root},nil
}
func (r FabricChaincodeDeployReconciler) legacyGetCryptoMaterial(ctx context.Context, labels map[string]string, ns string, fabricChaincode *hlfv1alpha1.FabricChaincode) (*SecretChaincodeData, error) {''')
    (build/'controllers/chaincode/deploy/smp_candidate_test.go').write_text((HERE/'chaincode_test.go.tmpl').read_text(),encoding='utf-8')
    for package in ('peer','ordnode','chaincode/deploy'):
        name=package.split('/')[-1]
        (build/f'controllers/{package}/smp_fixture_test.go').write_text((HERE/'fixture_test.go.tmpl').read_text().replace('PACKAGE',name),encoding='utf-8')
    paths=[build/'main.go',helper]+[build/f'controllers/{p}' for p in ('peer','ordnode','ca','chaincode/deploy')]
    subprocess.run(['gofmt','-w']+list(map(str,paths)),check=True)

def main():
    p=argparse.ArgumentParser();p.add_argument('--source',required=True);p.add_argument('--build-dir',required=True);p.add_argument('--output',required=True);a=p.parse_args()
    source,build,output=map(lambda x:Path(x).resolve(),(a.source,a.build_dir,a.output))
    if output==source or source in output.parents:raise RuntimeError('Output cannot overwrite reference checkout')
    prepare(source,build)
    packages=['./controllers/smpsecurity','./controllers/peer','./controllers/ordnode','./controllers/ca','./controllers/chaincode/deploy']
    subprocess.run(['go','test',*packages,'-run','^TestSMPSecurity','-count=1','-v'],cwd=build,check=True)
    output.parent.mkdir(parents=True,exist_ok=True)
    env=dict(os.environ,GOOS='linux',GOARCH='amd64',CGO_ENABLED='0')
    subprocess.run(['go','vet','./controllers/smpsecurity'],cwd=build,check=True)
    subprocess.run(['go','build','-trimpath','-o',str(output),'.'],cwd=build,env=env,check=True)
    def digest(path):return hashlib.sha256(path.read_bytes()).hexdigest()
    inputs={p.name:digest(p) for p in sorted(HERE.iterdir()) if p.is_file() and (p.suffix in ('.py','.tmpl') or p.name=='Dockerfile')}
    changed=['main.go','controllers/smpsecurity/security.go','controllers/smpsecurity/registry.go','charts/hlf-ca/templates/deployment.yaml','controllers/ca/ca_controller.go','controllers/peer/peer_controller.go','controllers/ordnode/ordnode_controller.go','controllers/chaincode/deploy/chaincode_deploy_controller.go']
    report={'schema':'sathonate.candidate-build/v1','bevelCommit':PIN,'patchInputs':inputs,'patchedSources':{name:digest(build/name) for name in changed},'binarySHA256':digest(output),'fullFabricLifecycle':'BLOCKED','adopted':False}
    (build/'smp-candidate-build.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
    if output!=build/'controller-candidate':shutil.copyfile(output,build/'controller-candidate')
    print('Isolated fail-closed candidate built; full Fabric lifecycle still unqualified; registry bridge requires real integration evidence')
if __name__=='__main__':main()
