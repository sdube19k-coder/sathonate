"""Build the pinned Bevel CLI with namespace-only discovery and internal URLs.

The upstream checkout is never modified. git archive reads the exact commit;
all edits are applied to a dedicated build directory and fail on source drift.
No Fabric lifecycle or cryptographic code is replaced.
"""
import argparse
import os
from pathlib import Path
import subprocess
import tarfile

COMMIT = "cd236ccc61d1d159b3dadb4625d08bb5682a8451"
parser = argparse.ArgumentParser()
parser.add_argument("--source", required=True)
parser.add_argument("--build-dir", required=True)
parser.add_argument("--output", required=True)
args = parser.parse_args()
source, build, output = map(lambda value: Path(value).resolve(),
                            (args.source, args.build_dir, args.output))
if build == source or source in build.parents or build in source.parents:
    raise SystemExit("Use a separate dedicated build directory")
build.mkdir(parents=True, exist_ok=True)
archive = build / "source.tar"
subprocess.run(["git", "-C", str(source), "archive", COMMIT,
                "--format=tar", "--output", str(archive)], check=True)
with tarfile.open(archive) as package:
    package.extractall(build, filter="data")
path = build / "kubectl-hlf/cmd/helpers/hlf.go"
text = path.read_text(encoding="utf-8")
text = text.replace('"fmt"', '"fmt"\n "os"', 1)
for resource in ("FabricCAs", "FabricPeers", "FabricOrdererNodes"):
    old = f".{resource}(ns).List"
    if old not in text:
        raise SystemExit(f"Pinned source drift: {old}")
    text = text.replace(old, f".{resource}(workerNamespace(ns)).List")
patches = {
    "func GetCAPublicURL(clientset *kubernetes.Clientset, node hlfv1alpha1.FabricCA) (string, error) {":
        'if os.Getenv("SMP_WORKER_NAMESPACE")!="" {return GetCAPrivateURL(node),nil}',
    "func GetPeerPublicURL(clientset *kubernetes.Clientset, node hlfv1alpha1.FabricPeer) (string, error) {":
        'if os.Getenv("SMP_WORKER_NAMESPACE")!="" {return GetPeerPrivateURL(node),nil}',
    "func GetOrdererPublicURL(clientset *kubernetes.Clientset, node hlfv1alpha1.FabricOrdererNode) (string, error) {":
        'if os.Getenv("SMP_WORKER_NAMESPACE")!="" {return GetOrdererPrivateURL(node),nil}',
    "func GetAdminOrdererPublicURL(clientset *kubernetes.Clientset, node hlfv1alpha1.FabricOrdererNode) (string, error) {":
        'if os.Getenv("SMP_WORKER_NAMESPACE")!="" {return "https://"+node.Name+"."+node.Namespace+":7053",nil}',
    "func GetURLForCA(certAuth *ClusterCA) (string, error) {":
        'if os.Getenv("SMP_WORKER_NAMESPACE")!="" {return "https://"+GetCAPrivateURL(certAuth.Object),nil}',
}
for signature, body in patches.items():
    if text.count(signature) != 1:
        raise SystemExit(f"Pinned source drift: {signature}")
    text = text.replace(signature, signature + "\n " + body, 1)
text += '\nfunc workerNamespace(ns string)string {if scoped:=os.Getenv("SMP_WORKER_NAMESPACE");scoped!="" {return scoped};return ns}\n'
path.write_text(text, encoding="utf-8")
subprocess.run(["gofmt", "-w", str(path)], check=True)
utils = build / "controllers/utils/utils.go"
text = utils.read_text(encoding="utf-8").replace('"net"', '"net"\n "os"', 1)
for signature, body in {
    "func GetPublicIPKubernetes(clientSet *kubernetes.Clientset) (string, error) {":
        'if os.Getenv("SMP_WORKER_NAMESPACE")!="" {return "127.0.0.1",nil}',
    "func GetPublicIPsKubernetes(clientSet *kubernetes.Clientset) ([]string, error) {":
        'if os.Getenv("SMP_WORKER_NAMESPACE")!="" {return []string{},nil}',
}.items():
    if text.count(signature) != 1:
        raise SystemExit(f"Pinned source drift: {signature}")
    text = text.replace(signature, signature + "\n " + body, 1)
utils.write_text(text, encoding="utf-8")
subprocess.run(["gofmt", "-w", str(utils)], check=True)
# Private stdin argument transport. No secret is added to kernel argv or env.
# Keep upstream cryptography and command implementations unchanged.
main = build / "kubectl-hlf/main.go"
text = main.read_text(encoding="utf-8")
text = text.replace('"os"', '\"os\"\n \"encoding/json\"\n \"io\"\n \"fmt\"\n \"strings\"', 1)
old = 'if err := cmd.NewCmdHLF().Execute(); err != nil {'
if text.count(old) != 1:
    raise SystemExit("Pinned main source drift")
text = text.replace(old, r'''if len(os.Args)==2 && os.Args[1]=="--smp-secret-input-version" {fmt.Println("smp-secret-input-v1");return}
    root := cmd.NewCmdHLF()
    protected := len(os.Args)==2 && os.Args[1]=="--smp-stdin-arguments"
    if protected {
        var args []string
        decoder:=json.NewDecoder(io.LimitReader(os.Stdin,(1<<20)+1))
        if decoder.Decode(&args)!=nil {fmt.Fprintln(os.Stderr,"invalid protected input");os.Exit(1)}
        var extra any
        if decoder.Decode(&extra)!=io.EOF {fmt.Fprintln(os.Stderr,"invalid protected input");os.Exit(1)}
        root.SetArgs(args)
        root.SilenceErrors=true
        root.SilenceUsage=true
        logrus.SetOutput(io.Discard)
    }
    if err := root.Execute(); err != nil {
        if protected {
            message:=strings.ToLower(err.Error())
            safe:="protected command failed"
            for _,marker:=range []string{"already registered","not found","not defined","does not exist","could not find","sequence: '0'"} {if strings.Contains(message,marker){safe=marker;break}}
            fmt.Fprintln(os.Stderr,safe)
        }''', 1)
main.write_text(text,encoding="utf-8")
# Enrollment files include private keys. Upstream's 0644 is inappropriate here.
enroll = build / "kubectl-hlf/cmd/ca/enroll.go"
text = enroll.read_text(encoding="utf-8")
if "userYaml, 0644" not in text:
    raise SystemExit("Pinned enrollment output source drift")
enroll.write_text(text.replace("userYaml, 0644", "userYaml, 0600"),encoding="utf-8")
subprocess.run(["gofmt","-w",str(main),str(enroll)],check=True)

output.parent.mkdir(parents=True, exist_ok=True)
env = dict(os.environ, GOOS="linux", GOARCH="amd64", CGO_ENABLED="0")
subprocess.run(["go", "build", "-trimpath", "-o", str(output), "./kubectl-hlf"],
               cwd=build, env=env, check=True)
print(f"Built scoped Bevel CLI from {COMMIT}: {output}")
