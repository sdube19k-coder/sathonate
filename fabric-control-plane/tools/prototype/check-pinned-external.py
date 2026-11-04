"""Test upstream validators in a separate pinned archive; never changes cluster/reference checkout."""
import argparse,subprocess,tarfile
from pathlib import Path
COMMIT='cd236ccc61d1d159b3dadb4625d08bb5682a8451'
p=argparse.ArgumentParser();p.add_argument('--source',required=True);p.add_argument('--build-dir',required=True);a=p.parse_args()
source=Path(a.source).resolve();build=Path(a.build_dir).resolve()
if source==build or source in build.parents or build in source.parents:raise SystemExit('Separate build directory required')
build.mkdir(parents=True,exist_ok=True);archive=build/'source.tar'
subprocess.run(['git','-C',str(source),'archive',COMMIT,'--format=tar','--output',str(archive)],check=True)
with tarfile.open(archive) as f:f.extractall(build,filter='data')
for package,kind in [('peer','FabricPeer'),('ordnode','FabricOrdererNode')]:
 text=f'''package {package}
import("testing"; hlf "github.com/kfsoftware/hlf-operator/pkg/apis/hlf.kungfusoftware.es/v1alpha1")
func TestSMPExternalCertificateWithoutPasswordBlocked(t *testing.T){{
 resource:=&hlf.{kind}{{}}
 {"resource.Spec.Secret=&hlf.Secret{}" if package=="ordnode" else ""}
 resource.Spec.Secret.Enrollment.Component.Cahost="ca.disposable"
 resource.Spec.Secret.Enrollment.Component.Enrollid="preenrolled"
 resource.Spec.Secret.Enrollment.Component.External=&hlf.ExternalCertificate{{SecretName:"preenrolled",SecretNamespace:"smp-prototype",CertificateKey:"cert.pem",PrivateKeyKey:"key.pem",RootCertificateKey:"root.pem"}}
 if (&ConfigValidator{{}}).validateFabricCAConfig(resource)==nil{{t.Fatal("unexpected pinned compatibility change")}}
 t.Log("Pinned Kubernetes-store validator rejects external certificate references without enrollment password")
}}
'''
 (build/'controllers'/package/'smp_external_prototype_test.go').write_text(text)
subprocess.run(['go','test','./controllers/peer','./controllers/ordnode','-run','^TestSMPExternalCertificateWithoutPasswordBlocked$','-count=1','-v'],cwd=build,check=True)
# Minimal validation-only candidate, exclusively in the disposable archive.
# This is NOT a complete controller migration and must never be installed as such.
for package,kind,variable in [('peer','FabricPeer','peer'),('ordnode','FabricOrdererNode','node')]:
 path=build/'controllers'/package/(package+'_controller.go')
 original=path.read_text()
 marker=f'func (v *ConfigValidator) validateFabricCAConfig({variable} *hlfv1alpha1.{kind}) error {{'
 if original.count(marker)!=1:raise SystemExit('Pinned validator source drift')
 candidate=f"""
 if {variable}.Spec.Secret.Enrollment.Component.External != nil || {variable}.Spec.Secret.Enrollment.TLS.External != nil {{
  for _, external := range []*hlfv1alpha1.ExternalCertificate{{{variable}.Spec.Secret.Enrollment.Component.External, {variable}.Spec.Secret.Enrollment.TLS.External}} {{
   if external == nil || external.SecretNamespace != {variable}.Namespace || external.SecretName == \"\" || external.CertificateKey == \"\" || external.PrivateKeyKey == \"\" || external.RootCertificateKey == \"\" {{return errors.New(\"same-namespace complete external certificate references required\")}}
  }}
  return nil
 }}
"""
 path.write_text(original.replace(marker,marker+candidate))
 test=build/'controllers'/package/'smp_external_prototype_test.go'
 body=test.read_text().replace('TestSMPExternalCertificateWithoutPasswordBlocked','TestSMPExternalCertificateValidationCandidate')
 body=body.replace('resource.Spec.Secret.Enrollment.Component.Cahost=', 'resource.Namespace="smp-prototype"\n resource.Spec.Secret.Enrollment.Component.Cahost=')
 start=body.index(' if (&ConfigValidator{}).validateFabricCAConfig')
 body=body[:start]+"""
 resource.Spec.Secret.Enrollment.TLS.External=resource.Spec.Secret.Enrollment.Component.External
 if (&ConfigValidator{}).validateFabricCAConfig(resource)!=nil{t.Fatal("complete external references rejected")}
 resource.Spec.Secret.Enrollment.TLS.External.SecretNamespace="foreign"
 if (&ConfigValidator{}).validateFabricCAConfig(resource)==nil{t.Fatal("cross-namespace reference accepted")}
 resource.Spec.Secret.Enrollment.TLS.External=nil
 if (&ConfigValidator{}).validateFabricCAConfig(resource)==nil{t.Fatal("incomplete references accepted")}
 t.Log("Validation-only candidate accepts complete same-namespace references and rejects foreign/incomplete references; reconciliation NOT VERIFIED")
}
"""
 test.write_text(body)
subprocess.run(['go','test','./controllers/peer','./controllers/ordnode','-run','^TestSMPExternalCertificateValidationCandidate$','-count=1','-v'],cwd=build,check=True)
print('Pinned rejection and isolated validation-only candidate tested; no controller replacement adopted')
