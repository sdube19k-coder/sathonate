"""Inspect Windows/WSL/Docker budgets without modifying any system setting."""
import argparse,configparser,hashlib,json,os,re,subprocess,sys
from datetime import datetime,timezone
from pathlib import Path
from preflight import metrics,assess,MIN_HOST_AVAILABLE

def configured_memory(text):
 parser=configparser.ConfigParser(interpolation=None)
 try:parser.read_string(text)
 except configparser.Error:return {'state':'UNPARSEABLE'}
 value=parser.get('wsl2','memory',fallback=None)
 if value is None:return {'state':'NOT_EXPLICITLY_CONFIGURED'}
 match=re.fullmatch(r'([1-9][0-9]*)\s*(KB|MB|GB|TB)',value.strip(),re.I)
 if not match:return {'state':'UNRECOGNIZED','explicit':True}
 return {'state':'EXPLICIT','value':value,'bytes':int(match[1])*1024**(['KB','MB','GB','TB'].index(match[2].upper())+1)}

def audit():
 report={'schema':'sathonate.sprint6-memory-audit/v1','checkedAt':datetime.now(timezone.utc).isoformat(),'settingsChanged':False,'wslRestarted':False,'dockerRestarted':False,'retainedNetworkModified':False}
 try:
  if os.name!='nt':raise RuntimeError('Windows/WSL2 memory inspection requires the Windows host')
  values=metrics();report['metrics']=values;report['capacity']=assess(values)
  command="$os=Get-CimInstance Win32_OperatingSystem;$pc=Get-CimInstance Win32_ComputerSystem;$vm=Get-Process -Name vmmemWSL -ErrorAction SilentlyContinue;@{physicalBytes=$pc.TotalPhysicalMemory;availableBytes=([long]$os.FreePhysicalMemory*1024);logicalCPUs=$pc.NumberOfLogicalProcessors;wslWorkingSetBytes=$vm.WorkingSet64;wslPrivateBytes=$vm.PrivateMemorySize64}|ConvertTo-Json -Compress"
  process=subprocess.run(['powershell.exe','-NoProfile','-NonInteractive','-Command',command],capture_output=True,text=True,timeout=30)
  if process.returncode:raise RuntimeError('Windows memory inventory unavailable')
  report['windows']=json.loads(process.stdout)
  path=Path.home()/'.wslconfig'
  if path.exists():
   raw=path.read_bytes();report['wslConfiguration']={**configured_memory(raw.decode('utf-8-sig')),'fileSHA256':hashlib.sha256(raw).hexdigest()}
  else:report['wslConfiguration']={'state':'FILE_ABSENT'}
  report['canSupplyBudgetWithoutSettingsChange']=report['capacity']['status']=='PASS_PREFLIGHT_ONLY'
  report['status']='READY_FOR_CAPACITY_GATED_TEST' if report['canSupplyBudgetWithoutSettingsChange'] else 'BLOCKED_CURRENT_LIMITS_OR_HEADROOM'
  report['hostAdditionalAvailableNeededBytes']=max(0,MIN_HOST_AVAILABLE-values['hostMemoryAvailable'])
  report['interpretation']='Current observations only. Installed RAM is not free RAM. A larger WSL cap alone does not prove safe physical headroom; the new fixture reservation must remain available with host margin.'
  report['configurationDocumentation']='https://learn.microsoft.com/en-us/windows/wsl/wsl-config'
 except Exception as error:
  report.update(status='BLOCKED_INSPECTION',failureCategory=str(error) if isinstance(error,RuntimeError) else type(error).__name__)
 return report

def main():
 parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--output',required=True);args=parser.parse_args();value=audit()
 with Path(args.output).open('x',encoding='utf-8') as output:json.dump(value,output,indent=2)
 print(value['status']+'; settings unchanged')
 return 0 if value['status']=='READY_FOR_CAPACITY_GATED_TEST' else 2
if __name__=='__main__':sys.exit(main())
