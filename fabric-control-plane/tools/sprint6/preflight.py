"""Read-only capacity gate and four-node plan; never creates or changes resources."""
import argparse,json,os,re,secrets,shutil,subprocess,sys
from datetime import datetime,timezone
from pathlib import Path
GIB=1024**3
# Conservative qualification reservation, not a measured Fabric requirement:
# 2.5 GiB Kubernetes/CNI + 2 GiB Fabric requests + 1 GiB worker + .5 GiB headroom.
# Keep a further 2 GiB outside the new fixture, including retained workloads.
MIN_TOTAL=8*GIB
MIN_AVAILABLE=6*GIB
MIN_DISK=40*GIB
# WSL free virtual pages need not already be physically resident in Windows.
# Reserve the entire new 6-GiB allocation plus 2 GiB for the host.
MIN_HOST_AVAILABLE=MIN_AVAILABLE+2*GIB

def assess(metrics):
 failures=[]
 for field,minimum in [('dockerCPUs',4),('dockerMemoryTotal',MIN_TOTAL),('dockerMemoryAvailable',MIN_AVAILABLE),('hostMemoryAvailable',MIN_HOST_AVAILABLE),('backingDiskFree',MIN_DISK)]:
  value=metrics.get(field)
  if type(value) is not int or value<minimum:failures.append({'metric':field,'observed':value,'required':minimum})
 return {'status':'PASS_PREFLIGHT_ONLY' if not failures else 'BLOCKED_CAPACITY','failures':failures,'policy':{'estimatedNewFixtureMemoryBytes':MIN_AVAILABLE,'minimumDockerMemoryBytes':MIN_TOTAL,'minimumBackingDiskFreeBytes':MIN_DISK,'minimumHostAvailableBytes':MIN_HOST_AVAILABLE,'estimatesAreNotRuntimeEvidence':True}}

def cluster_plan(name):
 if not re.fullmatch(r'smp-sprint1-[a-f0-9]{8}',name):raise ValueError('Only a fresh disposable qualification name is allowed')
 return {'kind':'Cluster','apiVersion':'kind.x-k8s.io/v1alpha4','name':name,'networking':{'disableDefaultCNI':True,'podSubnet':'192.168.0.0/16'},'nodes':[{'role':'control-plane'}]+[{'role':'worker','labels':{'fabric.sathonate.io/qualification-worker':str(i)}} for i in range(1,4)]}

def worker_inventory(name):
 cluster_plan(name)
 nodes=[name+'-worker',name+'-worker2',name+'-worker3']
 return {'workerNodes':nodes,'nodeSelectors':{n:{'kubernetes.io/hostname':n} for n in nodes}}

def command(args):
 p=subprocess.run(args,capture_output=True,text=True,timeout=30)
 if p.returncode:raise RuntimeError('Read-only capacity command failed: '+Path(args[0]).name)
 return p.stdout

def local_endpoint(endpoint,platform):
 return endpoint in ('npipe:////./pipe/dockerDesktopLinuxEngine','npipe:////./pipe/docker_engine') if platform=='nt' else endpoint.startswith('unix:///')

def local_engine_endpoint():
 override=os.environ.get('DOCKER_HOST','')
 if override and os.environ.get('DOCKER_CONTEXT'):raise RuntimeError('Ambiguous Docker host/context overrides refused')
 if override:
  if not local_endpoint(override,os.name):raise RuntimeError('Remote Docker host override refused')
  return override
 endpoint=command(['docker','context','inspect','--format','{{.Endpoints.docker.Host}}']).strip()
 if not local_endpoint(endpoint,os.name):raise RuntimeError('Local Docker endpoint required; remote capacity cannot be inferred from this host')
 return endpoint

def metrics():
 local_engine_endpoint()
 docker=json.loads(command(['docker','info','--format','{"cpus":{{.NCPU}},"memory":{{.MemTotal}}}']))
 if os.name=='nt':
  import ctypes
  class Memory(ctypes.Structure):
   _fields_=[('length',ctypes.c_ulong),('load',ctypes.c_ulong)]+[(n,ctypes.c_ulonglong) for n in ('total','available','pageTotal','pageAvailable','virtualTotal','virtualAvailable','extended')]
  m=Memory();m.length=ctypes.sizeof(m)
  if not ctypes.windll.kernel32.GlobalMemoryStatusEx(ctypes.byref(m)):raise RuntimeError('Host memory unavailable')
  host_available=int(m.available)
  raw=command(['wsl','-d','docker-desktop','--','cat','/proc/meminfo'])
  available=int(re.search(r'^MemAvailable:\s+(\d+) kB',raw,re.M).group(1))*1024
  disk=shutil.disk_usage(os.environ.get('SystemDrive','C:')+'/').free
 else:
  raw=Path('/proc/meminfo').read_text();available=int(re.search(r'^MemAvailable:\s+(\d+) kB',raw,re.M).group(1))*1024
  host_available=available;disk=shutil.disk_usage('/var/lib/docker' if Path('/var/lib/docker').exists() else '/').free
 return {'dockerCPUs':int(docker['cpus']),'dockerMemoryTotal':int(docker['memory']),'dockerMemoryAvailable':available,'hostMemoryAvailable':host_available,'backingDiskFree':int(disk)}

def main():
 parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--output',required=True);parser.add_argument('--kind-config');args=parser.parse_args()
 report={'schema':'sathonate.sprint6-preflight/v1','checkedAt':datetime.now(timezone.utc).isoformat(),'createdResources':False,'retainedNetworkModified':False,'liveAcceptance':'NOT_TESTED'}
 try:
  values=metrics();report.update(assess(values));report['metrics']=values
 except Exception as error:
  report.update(status='BLOCKED_ENVIRONMENT',failureCategory=str(error) if isinstance(error,RuntimeError) else type(error).__name__)
 name='smp-sprint1-'+secrets.token_hex(4);report['plannedInventory']=worker_inventory(name)
 if args.kind_config:
  with Path(args.kind_config).open('x',encoding='utf-8') as output:json.dump(cluster_plan(name),output,indent=2)
 with Path(args.output).open('x',encoding='utf-8') as output:json.dump(report,output,indent=2)
 print(report['status']+'; no Kubernetes or Docker resources created')
 return 0 if report['status']=='PASS_PREFLIGHT_ONLY' else 2
if __name__=='__main__':sys.exit(main())
