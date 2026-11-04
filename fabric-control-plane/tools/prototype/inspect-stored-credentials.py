"""Read-only retained CR inspection. Emits field paths/counts only, never values."""
import argparse,json,subprocess
from datetime import datetime,timezone
p=argparse.ArgumentParser();p.add_argument('--kubeconfig',required=True);p.add_argument('--context',required=True);p.add_argument('--namespace',required=True);p.add_argument('--output',required=True);a=p.parse_args()
base=['kubectl','--kubeconfig',a.kubeconfig,'--context',a.context]
result={'capturedAt':datetime.now(timezone.utc).isoformat(),'passwordFree':False,'inspection':'read-only actual stored Fabric resources','resources':[]}
for kind in ('fabriccas','fabricpeers','fabricorderernodes','fabricchaincodes'):
 r=subprocess.run(base+['-n',a.namespace,'get',kind,'-o','json'],capture_output=True,text=True)
 if r.returncode:raise SystemExit('Read-only Fabric CR inspection failed; diagnostics withheld')
 for item in json.loads(r.stdout)['items']:
  paths=[]
  def walk(x,path):
   if isinstance(x,dict):
    for k,v in x.items():
     q=path+'.'+k
     if k.lower() in ('pass','password','enrollsecret','secretpassword') and isinstance(v,str) and v:paths.append(q)
     else:walk(v,q)
   elif isinstance(x,list):
    for i,v in enumerate(x):walk(v,path+'['+str(i)+']')
  walk(item.get('spec',{}),'spec')
  result['resources'].append({'kind':item['kind'],'name':item['metadata']['name'],'populatedCredentialFieldPaths':paths})
result['passwordFree']=bool(result['resources']) and all(not x['populatedCredentialFieldPaths'] for x in result['resources'])
from pathlib import Path
Path(a.output).write_text(json.dumps(result,indent=2)+'\n')
print('Read-only credential field-path evidence written; no credential values exported')
