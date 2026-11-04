import ast,json,tempfile,unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch
import lifecycle

class LifecycleSafetyTests(unittest.TestCase):
 def lab(self,tmp):return lifecycle.Lifecycle(SimpleNamespace(cache=tmp,kind='kind',source='reference',image='candidate',output=str(Path(tmp)/'evidence.json')))
 def test_embedded_participation_and_sampler_programs_compile(self):
  compile(lifecycle.PARTICIPATION_SCRIPT,'participation.py','exec')
  compile(lifecycle.IDENTITY_SCRIPT,'identity.py','exec')
  tree=ast.parse(Path(lifecycle.__file__).read_text())
  sampler=next(n.value for n in ast.walk(tree) if isinstance(n,ast.Assign) and any(isinstance(t,ast.Name) and t.id=='sampler' for t in n.targets))
  compile(ast.literal_eval(sampler),'sampler.py','exec')
 def test_inline_credential_never_reaches_public_api(self):
  with tempfile.TemporaryDirectory() as tmp:
   lab=self.lab(tmp);lab.passwords=['private-canary']
   with patch.object(lab,'kub') as api:
    with self.assertRaises(RuntimeError):lab.apply({'kind':'FabricPeer','spec':{'enrollsecret':'private-canary'}})
    api.assert_not_called()
 def test_failed_operation_exports_categories_not_diagnostics(self):
  with tempfile.TemporaryDirectory() as tmp:
   lab=self.lab(tmp)
   with patch.object(lab,'kub',return_value=SimpleNamespace(returncode=1,stderr='HTTP Error 400 private-canary',stdout='private-key-canary')):
    with self.assertRaises(RuntimeError) as failure:lab.exec('tools','python','-c','pass')
   self.assertNotIn('canary',str(failure.exception)+json.dumps(lab.result))
   self.assertEqual(lab.result['operationFailure']['categories'],['HTTP Error 400'])
 def test_peer_operations_use_explicit_role_msp_without_credentials_in_arguments(self):
  with tempfile.TemporaryDirectory() as tmp:
   lab=self.lab(tmp);lab.cas={'org1':{'status':{'tlsca_cert':'public-tls-root','ca_cert':'public-signing-root'}}}
   with patch.object(lab,'write') as write,patch.object(lab,'exec',return_value='ok') as execute:
    self.assertEqual(lab.peer('org1','channel','join','-b','/workspace/genesis.block',identity='chaincode-admin'),'ok')
   writes=dict(call.args for call in write.call_args_list)
   msp='/workspace/org1-chaincode-admin/msp'
   self.assertEqual(writes[msp+'/cacerts/identity.pem'],'public-signing-root')
   config=lifecycle.yaml.safe_load(writes[msp+'/config.yaml'])['NodeOUs']
   self.assertTrue(config['Enable'])
   self.assertEqual(config['AdminOUIdentifier'],{'Certificate':'cacerts/identity.pem','OrganizationalUnitIdentifier':'admin'})
   self.assertIn('CORE_PEER_MSPCONFIGPATH='+msp,execute.call_args.args)
   self.assertFalse(any('password' in arg or 'PRIVATE KEY' in arg for arg in execute.call_args.args))
 def test_chaincode_resource_uses_pinned_server_port_and_external_credentials(self):
  with tempfile.TemporaryDirectory() as tmp:
   lab=self.lab(tmp);lab.cas={'org1':{'status':{'tlsca_cert':'public-root','tls_cert':'public-server'}}}
   with patch.object(lab,'exec'),patch.object(lab,'apply') as apply,patch.object(lab,'wait'),patch.object(lab,'peer'),patch.object(lab,'transactions'):
    lab.contract()
   resource=apply.call_args.args[0];spec=resource['spec']
   self.assertEqual(spec['chaincodeServerPort'],7052);self.assertNotIn('port',spec)
   self.assertEqual(spec['credentials']['enrollsecret'],'')
   self.assertEqual(spec['credentials']['external']['secretName'],'smp-smoke-external')
 def test_unreadable_or_credential_bearing_runtime_logs_cannot_pass(self):
  for code,body in ((1,''),(0,'private-canary')):
   with self.subTest(code=code),tempfile.TemporaryDirectory() as tmp:
    lab=self.lab(tmp);lab.passwords=['private-canary']
    responses=[SimpleNamespace(stdout='{"items":[]}'),SimpleNamespace(stdout='{"items":[{"metadata":{"name":"fixture"},"spec":{"containers":[{"name":"fixture"}]}}]}'),SimpleNamespace(returncode=code,stdout=body)]
    with patch.object(lab,'kub',side_effect=responses):
     with self.assertRaises(RuntimeError) as failure:lab.audit_logs('runtime-logs')
    self.assertNotIn('private-canary',str(failure.exception)+json.dumps(lab.result))
    self.assertNotIn('runtime-logs',lab.result['checks'])
if __name__=='__main__':unittest.main()
