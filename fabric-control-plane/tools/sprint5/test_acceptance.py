import io,secrets,unittest,urllib.error
from unittest.mock import patch
from acceptance import local_api

class AcceptanceTransport(unittest.TestCase):
 def test_deletion_transport_uses_selected_server_bound_token(self):
  tokens={k:secrets.token_urlsafe(32) for k in ('primary','administrator')}
  class Response(io.BytesIO):
   status=200
  with patch('acceptance.urllib.request.urlopen',return_value=Response(b'{"removed":true}')) as opened:
   status,body=local_api(tokens)('/deployments/owned',token=tokens['administrator'],method='DELETE',headers={'X-Sathonate-Cleanup-Approval':'a'*64})
  request=opened.call_args.args[0]
  self.assertEqual(request.get_header('Authorization'),'Bearer '+tokens['administrator'])
  self.assertEqual(request.method,'DELETE');self.assertEqual(status,200);self.assertTrue(body['removed'])
  self.assertEqual(request.full_url,'http://127.0.0.1:8080/api/v1/deployments/owned')
  self.assertNotIn(tokens['administrator'],request.full_url)
 def test_api_rejection_remains_a_rejection(self):
  tokens={'primary':secrets.token_urlsafe(32)}
  failure=urllib.error.HTTPError('http://127.0.0.1:8080',409,'Conflict',{},io.BytesIO(b'{"error":{"code":"cleanup"}}'))
  with patch('acceptance.urllib.request.urlopen',side_effect=failure):
   status,body=local_api(tokens)('/deployments/owned')
  self.assertEqual(status,409);self.assertEqual(body['error']['code'],'cleanup')

if __name__=='__main__':unittest.main()
