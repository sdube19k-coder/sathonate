"""Bounded worker-pause acceptance state machine; adapters own all side effects."""
import copy,time

class Failure(RuntimeError):
 def __init__(self,code):self.code=code;super().__init__(code)

def require(value,code):
 if not value:raise Failure(code)

def candidates(network,workers,components):
 """A candidate must leave an actual majority for EVERY customer channel."""
 require(len(workers)==3 and len(set(workers))==3,'worker-inventory')
 require(bool(network['spec']['channels']),'channel-inventory-empty')
 nodes={n['id']:n for n in network['spec']['nodes']}
 result=[]
 for worker in workers:
  lost={name for name,c in components.items() if c['node']==worker}
  roles={nodes[name]['role'] for name in lost if name in nodes}
  authorities={a['id'] for a in network['spec']['configuration']['authorities']}
  if not {'peer','orderer'}.issubset(roles) or not lost.intersection(authorities):continue
  if not any(set(c['ordererRefs']).intersection(lost) for c in network['spec']['channels']):continue
  budgets=[]
  for channel in network['spec']['channels']:
   orders=channel['ordererRefs'];remaining=[n for n in orders if n not in lost]
   if any(n not in components or nodes.get(n,{}).get('role')!='orderer' for n in orders) or len(set(orders))!=len(orders) or len(orders)<3 or len(orders)%2!=1 or len(remaining)<len(orders)//2+1:break
   budgets.append({'channel':channel['id'],'required':len(orders)//2+1,'remaining':remaining})
  else:
   result.append({'worker':worker,'channels':budgets,'lost':sorted(lost)})
 require(result,'no-safe-worker-with-quorum')
 return sorted(result,key=lambda c:(len(c['lost']),c['worker']))

def pv_host(affinity):
 terms=affinity.get('required',{}).get('nodeSelectorTerms',[])
 require(bool(terms),'local-pv-affinity-missing')
 hosts=[]
 for term in terms:
  bound=[e for e in term.get('matchExpressions',[]) if e.get('key')=='kubernetes.io/hostname' and e.get('operator')=='In' and len(e.get('values',[]))==1]
  require(len(bound)==1,'local-pv-affinity-ambiguous');hosts+=bound[0]['values']
 require(len(set(hosts))==1,'local-pv-affinity-multiple-hosts')
 return hosts[0]

def unchanged_storage(before,after):
 require(bool(before) and set(before)==set(after),'pvc-inventory-changed')
 for name,old in before.items():
  new=after[name]
  require(old.get('phase')=='Bound','pvc-preflight-not-bound')
  require(new.get('phase')=='Bound','pvc-not-bound')
  require(all(old.get(k) and old[k]==new.get(k) for k in ('uid','pv','pvUID','host')),'pvc-or-pv-replaced')

def checked(result,names):
 require(all(result.get(name) is True for name in names),'acceptance-check-incomplete')
 return result

OUTAGE=['nodeNotReadyObserved','monitoringDegraded','statelessRescheduled','localPVProbeUnschedulable','originalPVCsBoundDuringOutage','raftHealthyMajorityViaMutualTLS','crossWorkerTLS']
CLEANUP=['workerUnpaused','uidCheckedResourcesRemoved','privateGrantsRemoved','apiStopped','credentialInspectionClean','pauseBounded']
RECOVERY=['sameWorkerRecovered','PVCAndPVUIDsPreserved','immutableIdentitiesPreserved','priorTransactionsVALID','freshAPITransactionsCommitted','allPeerLedgersConverged','recoveryIdempotent','monitoringReady']

def run_case(io,save,clock=time.monotonic):
 receipt={'schema':'sathonate.sprint6-worker-failure/v1','status':'IN_PROGRESS','checks':{},'durationsSeconds':{},'limits':['Single-PC kind; bounded worker pause before normal stateful eviction.','Stateless rescheduling is exercised; stateful local PVs must not migrate.','No write is submitted during the outage; committed history and fresh API transactions are checked after recovery.']}
 intent=False;worker=None;boundary='prepare'
 def checkpoint(stage):
  receipt['stage']=stage;save(copy.deepcopy(receipt))
 try:
  baseline=io.prepare();worker=baseline['worker'];receipt['baseline']=baseline
  boundary='authorization'
  receipt['checks'].update(checked(io.authorize(),['authorization']))
  checkpoint('PREFLIGHT_COMPLETE');intent=True;start=clock()
  boundary='pause';checkpoint('PAUSE_REQUESTED');io.pause(worker);checkpoint('PAUSE_ACKNOWLEDGED')
  boundary='observe_outage'
  receipt['checks'].update(checked(io.observe_outage(worker),OUTAGE))
  receipt['durationsSeconds']['outageObservation']=clock()-start;checkpoint('OUTAGE_VERIFIED')
  boundary='resume';checkpoint('RESUME_REQUESTED');start=clock();io.resume(worker);intent=False;checkpoint('RESUME_ACKNOWLEDGED')
  boundary='recover';recovered=io.recover(worker)
  receipt['checks'].update(checked(recovered['checks'],RECOVERY))
  receipt['recovery']=recovered['evidence'];receipt['durationsSeconds']['recoveryIncludingNewTransactions']=clock()-start
  checkpoint('RECOVERY_VERIFIED');receipt['status']='PASS_BOUNDED_WORKER_PAUSE_RECOVERY'
 except Exception as error:
  receipt['status']='FAIL';receipt['failedBoundary']=boundary;receipt['failureCategory']=error.code if isinstance(error,Failure) else type(error).__name__
 finally:
  if intent:
   try:io.resume(worker);receipt['cleanupResume']='ACKNOWLEDGED'
   except Exception:receipt['cleanupResume']='FAILED';receipt['status']='FAIL'
  try:
   cleanup=io.cleanup();receipt['cleanup']=cleanup
   require(all(cleanup.get(name) is True for name in CLEANUP),'cleanup-incomplete')
  except Exception as error:
   receipt['status']='FAIL';receipt['cleanupFailure']=error.code if isinstance(error,Failure) else type(error).__name__
  checkpoint('FINISHED')
 return receipt


def cross_probe_workers(workers,peer_node):
 require(len(workers)==3 and len(set(workers))==3 and peer_node in workers,'cross-worker-probe-inventory')
 return [node for node in workers if node!=peer_node]


def completed_case(value):
 return value.get('status')=='PASS_BOUNDED_WORKER_PAUSE_RECOVERY' and all(value.get('checks',{}).get(name) is True for name in ['authorization']+OUTAGE+RECOVERY) and all(value.get('cleanup',{}).get(name) is True for name in CLEANUP)
