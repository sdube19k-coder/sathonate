import ast,hashlib,re,tempfile,types,unittest
from pathlib import Path
class Loading(unittest.TestCase):
 def test_archive_and_controller_pins_exist_on_every_owned_node(self):
  source=Path(__file__).resolve().parents[1]/'sprint1/qualify.py'
  tree=ast.parse(source.read_text(encoding='utf-8'));functions=[n for n in tree.body if isinstance(n,ast.FunctionDef) and n.name in ('load','image_nodes')]
  for multi in [False,True]:
   with self.subTest(multi=multi),tempfile.TemporaryDirectory() as directory:
    calls=[];imports=[];digest='sha256:'+'d'*64;identity='sha256:'+'a'*64
    archive=Path(directory)/('sprint1-public-'+identity.split(':')[1]+'.tar');archive.write_bytes(b'fixture-archive');archive.with_suffix('.sha256').write_text(hashlib.sha256(archive.read_bytes()).hexdigest())
    def run(args,**kwargs):
     args=list(map(str,args));calls.append(args)
     if args[:3]==['docker','image','inspect']:return identity
     if args[-2:]==['images','list']:return 'docker.io/example/worker:test application/vnd.oci.image.manifest.v1+json '+digest+' x\ndocker.io/sathonate/controller-candidate:atomic application/vnd.oci.image.manifest.v1+json '+digest+' x'
     return ''
    def imported(args,**kwargs):imports.append(list(map(str,args)));self.assertEqual(kwargs['stdin'].read(),b'fixture-archive');return types.SimpleNamespace(returncode=0)
    namespace={'NAME':'smp-sprint1-a1b2c3d4','MULTI':multi,'CACHE':Path(directory),'KIND':Path('kind'),'run':run,'subprocess':types.SimpleNamespace(run=imported),'hashlib':hashlib,'re':re,'tarfile':None}
    exec(compile(ast.Module(body=functions,type_ignores=[]),str(source),'exec'),namespace)
    pin,_=namespace['load']('example/worker:test');self.assertEqual(pin,'example/worker@'+digest)
    expected=namespace['image_nodes']();self.assertEqual([args[3] for args in imports],expected)
    for node in expected:self.assertTrue(any(args[:3]==['docker','exec',node] and 'tag' in args for args in calls))
    calls.clear();namespace['load']('sathonate/controller-candidate:atomic')
    for node in expected:self.assertTrue(any(args[:3]==['docker','exec',node] and 'tag' in args for args in calls))
if __name__=='__main__':unittest.main()
