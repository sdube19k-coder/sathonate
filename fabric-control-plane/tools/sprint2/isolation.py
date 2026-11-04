"""Separate disposable Calico fixture. No existing cluster is selected or changed."""
import hashlib,json,re,subprocess,urllib.request
from pathlib import Path
VERSION='v3.31.4'
URL='https://raw.githubusercontent.com/projectcalico/calico/'+VERSION+'/manifests/calico.yaml'
SHA256='6e89ee080d6dae58c620c47e3fcb7a9bfc40748422c868c924b7482dd5145ce2'
def install(run,kub,apply,load,kind,name,cache,report):
 path=cache/('calico-'+VERSION+'.yaml')
 if not path.exists():path.write_bytes(urllib.request.urlopen(URL,timeout=30).read())
 data=path.read_bytes()
 if hashlib.sha256(data).hexdigest()!=SHA256:raise RuntimeError('Calico fixture manifest provenance mismatch')
 text=data.decode();images={}
 for image in sorted(set(re.findall(r'image: (quay.io/calico/[^\s]+)',text))):
  run(['docker','pull',image],timeout=180)
  metadata=json.loads(run(['docker','image','inspect',image]))[0]
  pinned=next((x for x in metadata.get('RepoDigests',[]) if x.startswith(image.split(':')[0]+'@sha256:')),None)
  if not pinned:raise RuntimeError('Calico image immutable provenance unavailable')
  pinned,_=load(image)
  text=text.replace('image: '+image,'image: '+pinned);images[image]=pinned
 kub('apply','--server-side','-f','-',data=text)
 kub('-n','kube-system','rollout','status','daemonset/calico-node','--timeout=180s')
 kub('-n','kube-system','rollout','status','deployment/calico-kube-controllers','--timeout=180s')
 kub('wait','node',name+'-control-plane','--for=condition=Ready','--timeout=120s')
 report['networkPolicyFixture']={'provider':'Calico','version':VERSION,'manifestURL':URL,'manifestSHA256':SHA256,'images':images,'enforcement':'NOT VERIFIED'}

def inspect(kub,apply,admission_denied,namespace,worker,report):
 foreign=report['cluster']+'-foreign'
 # A foreign pod and an own-namespace observer must run identical probes. Own
 # success excludes a broken peer/listener as the reason a foreign probe fails.
 pod=lambda ns,name: {'apiVersion':'v1','kind':'Pod','metadata':{'name':name,'namespace':ns},'spec':{'automountServiceAccountToken':False,'restartPolicy':'Never','containers':[{'name':'probe','image':worker,'imagePullPolicy':'Never','command':['python','-c','import time;time.sleep(600)'],'securityContext':{'runAsNonRoot':True,'runAsUser':10001,'allowPrivilegeEscalation':False,'capabilities':{'drop':['ALL']},'seccompProfile':{'type':'RuntimeDefault'}},'resources':{'requests':{'cpu':'25m','memory':'32Mi'},'limits':{'cpu':'100m','memory':'128Mi'}}}]}}
 privileged=pod(namespace,'privileged-denial');privileged['spec']['containers'][0]['securityContext']={'privileged':True}
 admission_denied(privileged,'violates PodSecurity')
 excessive=pod(namespace,'limit-denial');excessive['spec']['containers'][0]['resources']['limits']['cpu']='3'
 admission_denied(excessive,'maximum cpu')
 report['checks'].update(privilegedAdmissionDenied='PASS',containerLimitAdmissionDenied='PASS')
 for ns in (namespace,foreign):
  apply(pod(ns,'isolation-probe'));kub('-n',ns,'wait','pod/isolation-probe','--for=condition=Ready','--timeout=90s')
 script='import socket;socket.create_connection(("org1-peer-1.'+namespace+'",7051),timeout=3).close();print("CONNECTED")'
 own=kub('-n',namespace,'exec','isolation-probe','--','python','-c',script).strip()
 if own!='CONNECTED':raise RuntimeError('Own Fabric network connectivity unverified')
 # Exception is handled inside the probe: kubectl success alone is insufficient.
 blocked='import socket;\ntry:\n socket.create_connection(("org1-peer-1.'+namespace+'",7051),timeout=3).close();print("CONNECTED")\nexcept OSError:print("DENIED")'
 actual=kub('-n',foreign,'exec','isolation-probe','--','python','-c',blocked).strip()
 if actual!='DENIED':raise RuntimeError('Cross-namespace Fabric ingress is not isolated')
 # Egress denial to a live foreign listener, with a foreign positive control.
 server=pod(foreign,'egress-listener');server['spec']['containers'][0]['command']=['python','-m','http.server','7051'];apply(server)
 kub('-n',foreign,'wait','pod/egress-listener','--for=condition=Ready','--timeout=90s')
 ip=kub('-n',foreign,'get','pod','egress-listener','-o','jsonpath={.status.podIP}').strip()
 foreign_script=script.replace('org1-peer-1.'+namespace,ip)
 if kub('-n',foreign,'exec','isolation-probe','--','python','-c',foreign_script).strip()!='CONNECTED':raise RuntimeError('Foreign listener positive control failed')
 if kub('-n',namespace,'exec','isolation-probe','--','python','-c',blocked.replace('org1-peer-1.'+namespace,ip)).strip()!='DENIED':raise RuntimeError('Cross-namespace egress is not isolated')
 default=json.loads(kub('-n',namespace,'get','serviceaccount','default','-o','json'))
 if default.get('automountServiceAccountToken') is not False:raise RuntimeError('Default service-account token mount is not disabled')
 peers=json.loads(kub('-n',namespace,'get','pods','-o','json'))['items']
 for pod in peers:
  if pod['metadata']['name'].startswith(('org1-peer-1','org2-peer-1','orderer-','org1-ca','org2-ca')):
   if any(v.get('projected',{}).get('sources') and any('serviceAccountToken' in x for x in v['projected']['sources']) for v in pod['spec'].get('volumes',[])):raise RuntimeError('Fabric pod unexpectedly mounts an API token')
 report['checks'].update(effectiveCrossNamespaceIngressDenied='PASS',effectiveCrossNamespaceEgressDenied='PASS',ownFabricConnectivity='PASS',fabricServiceAccountTokenMountsAbsent='PASS',defaultServiceAccountTokenDisabled='PASS')
 report['networkPolicyFixture']['enforcement']='PASS'
