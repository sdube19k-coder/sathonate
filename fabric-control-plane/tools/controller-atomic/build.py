"""Separate atomic peer candidate. Never overwrites the verified lab build/image."""
import argparse,hashlib,json,os,shutil,subprocess,sys
from pathlib import Path
HERE=Path(__file__).resolve().parent
sys.path.insert(0,str(HERE.parent/'controller-lab'))
import build as baseline

def prepare(source,folder):
 baseline.prepare(source,folder)
 patch=baseline.replace
 helper=folder/'controllers/smpsecurity'
 patch(helper/'material.go','func ProtectChart(', 'func LegacyProtectChart(')
 patch(helper/'material_test.go','func TestSMPSecurityChartKeysNeverReachHelm(', 'func TestSMPSecurityLegacyChartKeysNeverReachHelm(')
 p=helper/'material_test.go';p.write_text(p.read_text().replace('ProtectChart(', 'LegacyProtectChart('))
 for name in ('atomic.go','atomic_test.go'):(helper/name).write_text((HERE/(name+'.tmpl')).read_text())
 peer=folder/'controllers/peer/peer_controller.go'
 patch(folder/'controllers/peer/types.go','type FabricPeerChart struct {','type FabricPeerChart struct {\n MaterialSecret string `json:"smpMaterialSecret"`\n MaterialUID string `json:"smpMaterialUID"`')
 # Status reads the bundle actually selected by the Deployment, not the desired
 # CR and never a partially updated set of conventional mutable Secrets.
 for function,role in [('getExistingTLSCrypto','tls'),('getExistingSignCrypto','sign')]:
  start=peer.read_text().index('func '+function+'(')
  text=peer.read_text();end=text.index('\n}\n',start)+3
  signature=text[start:text.index('{',start)+1]
  body='''
 data,e:=smpsecurity.ActiveMaterial(context.Background(),client,namespace,chartName)
 if e!=nil{return nil,nil,nil,e}
 cert,e:=utils.ParseX509Certificate(data["ROLE.cert"]);if e!=nil{return nil,nil,nil,e}
 key,e:=utils.ParseECDSAPrivateKey(data["ROLE.key"]);if e!=nil{return nil,nil,nil,e}
 root,e:=utils.ParseX509Certificate(data["ROLE.root"]);if e!=nil{return nil,nil,nil,e}
 return cert,key,root,nil
}
'''.replace('ROLE',role)
  peer.write_text(text[:start]+signature+body+text[end:])
 # Wrap only this peer controller's Helm client, using its existing Role.
 text=peer.read_text();i=text.index('func newActionCfg(')
 before,tail=text[:i],text[i:]
 needle='return cfg, err'
 if needle not in tail:raise RuntimeError('Pinned action configuration return changed')
 var=needle.split()[1].rstrip(',')
 tail=tail.replace(needle,f'''if err!=nil{{return nil,err}}
 client,e:=kubernetes.NewForConfig(clusterCfg);if e!=nil{{return nil,e}}
 {var}.KubeClient=smpsecurity.AtomicKubeClient{{Interface:{var}.KubeClient,Client:client,Namespace:namespace}}
 {needle}''',1);peer.write_text(before+tail)
 patch(peer,'\tcmd.Wait = r.Wait\n\tcmd.MaxHistory = r.MaxHistory','\tcmd.Wait = true\n\tcmd.MaxHistory = r.MaxHistory')
 patch(peer,'\trelease, err := cmd.Run(releaseName, ch, inInterface)', '''
 client,clientErr:=kubernetes.NewForConfig(r.Config);if clientErr!=nil{return clientErr}
 last,lastErr:=cfg.Releases.Last(releaseName)
 if lastErr==nil&&last.Info.Status==release.StatusDeployed&&smpsecurity.SamePublicChart(last.Config,inInterface){return smpsecurity.Ack(context.Background(),client,ns,releaseName,c.MaterialSecret,c.MaterialUID)}
 release, err := cmd.Run(releaseName, ch, inInterface)''')
 patch(peer,'\tr.Log.Info("Chart upgraded successfully", "release", release.Name)\n\treturn nil','''
 r.Log.Info("Chart upgraded successfully", "release", release.Name)
 return smpsecurity.Ack(context.Background(),client,ns,releaseName,c.MaterialSecret,c.MaterialUID)''')
 patch(peer,'\t} else if exists && helmStatus.Info.Status == release.StatusPendingRollback {','''
  helmStatus,err=cmdStatus.Run(releaseName);if err!=nil{return ctrl.Result{},err}
 } else if exists && helmStatus.Info.Status == release.StatusPendingRollback {''')
 patch(peer,'rollbackStatus.Version = helmStatus.Version - 1','''history,historyErr:=action.NewHistory(cfg).Run(releaseName);if historyErr!=nil{return ctrl.Result{},historyErr}
 previous,historyErr:=smpsecurity.RollbackRevision(history,helmStatus.Version);if historyErr!=nil{return ctrl.Result{},historyErr}
 rollbackStatus.Version = previous''')
 patch(folder/'main.go','ChartPath: "/charts/hlf-peer", Timeout: timeout, MaxHistory: 1','ChartPath: "/charts/hlf-peer", Timeout: timeout, MaxHistory: 5')
 # Each volume projects from the same immutable bundle. No mixed-generation
 # signing cert/key or mutable TLS files can be observed by a restarted pod.
 path=folder/'charts/hlf-peer/templates/deployment.yaml';text=path.read_text()
 start=text.index('        - name: id-cert\n');end=text.index('      {{- if .Values.externalChaincodeBuilder }}',start)
 volumes={'id-cert':{'sign.cert':'cert.pem'},'id-key':{'sign.key':'key.pem'},'cacert':{'sign.root':'cacert.pem'},'tls-clientrootcert':{'tls.root':'cert.pem'},'tls':{'tls.cert':'tls.crt','tls.key':'tls.key'},'tls-ops':{'ops.cert':'tls.crt','ops.key':'tls.key'},'tls-rootcert':{'tls.root':'cacert.pem'},'tls-client':{'tls.cert':'tls.crt','tls.key':'tls.key'}}
 out=''
 for name,items in volumes.items():
  out+='        - name: '+name+'\n          secret:\n            secretName: {{ required "validated material bundle required" .Values.smpMaterialSecret }}\n            defaultMode: 0440\n            items:\n'
  for key,value in items.items():out+='              - key: '+key+'\n                path: '+value+'\n'
 # Preserve the public NodeOU config volume; all certificate files are bundled.
 out+='        - name: nodeou\n          configMap:\n            name: \'{{ include "hlf-peer.fullname" . }}-nodeou\'\n            items:\n              - key: config.yaml\n                path: config.yaml\n'
 text=text[:start]+out+text[end:]
 text=text.replace('    metadata:\n      labels:','    metadata:\n      annotations:\n        sathonate.io/material: {{ .Values.smpMaterialSecret | quote }}\n        sathonate.io/material-owner: {{ .Values.smpMaterialUID | quote }}\n      labels:')
 path.write_text(text)
 for name in ('secret--peer-idcert.yaml','secret--peerorg-cacert.yaml','secret--tlsclientrootcert.yaml'):
  (path.parent/name).write_text('{{/* Public certificate is part of the immutable material bundle. */}}\n')
 subprocess.run(['gofmt','-w',str(helper),str(folder/'controllers/peer'),str(folder/'main.go')],check=True)

def main():
 p=argparse.ArgumentParser();p.add_argument('--source',required=True);p.add_argument('--build-dir',required=True);p.add_argument('--output',required=True);a=p.parse_args()
 source,folder,output=[Path(v).resolve() for v in (a.source,a.build_dir,a.output)]
 if folder.name!='controller-atomic-candidate' or output.name!='controller-atomic' or source in output.parents:raise RuntimeError('Separate atomic build and binary required')
 prepare(source,folder)
 packages=['./controllers/smpsecurity','./controllers/peer','./controllers/ordnode','./controllers/ca','./controllers/chaincode/deploy']
 subprocess.run(['go','test',*packages,'-run','^TestSMPSecurity','-count=1','-v'],cwd=folder,check=True)
 subprocess.run(['go','vet','.',*packages],cwd=folder,check=True)
 subprocess.run(['go','build','-trimpath','-o',str(output),'.'],cwd=folder,env=dict(os.environ,GOOS='linux',GOARCH='amd64',CGO_ENABLED='0'),check=True)
 def digest(p):return hashlib.sha256(p.read_bytes()).hexdigest()
 inputs={str(p.relative_to(HERE.parent)):digest(p) for d in (HERE,HERE.parent/'controller-lab') for p in d.iterdir() if p.is_file() and (p.suffix in ('.py','.tmpl') or p.name=='Dockerfile')}
 report={'schema':'sathonate.atomic-build/v1','bevelCommit':baseline.PIN,'baselineRevision':'5aa115c9b41a4384ef73a3ddf6fbfeb421e2ffe3','patchInputs':inputs,'binarySHA256':digest(output),'adopted':False}
 (folder/'smp-candidate-build.json').write_text(json.dumps(report,indent=2)+'\n');shutil.copyfile(output,folder/'controller-candidate')
 print('Separate atomic peer candidate compiled; runtime qualification required')
if __name__=='__main__':main()
