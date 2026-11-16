"""Deadline-capable cleanup for the disposable Sprint 3 EKS qualification."""
import argparse
import hashlib
import json
import re
import subprocess
import tempfile
import time
from contextlib import contextmanager
from datetime import datetime, timezone
from pathlib import Path

from teardown_stack import ACCOUNT, PROFILE, REGION, RUN, aws, teardown


class StorageTimeout(TimeoutError):
    pass


def remove_detached_run_volumes(call):
    tag = f"Name=tag:SathonateRun,Values={RUN}"
    volumes = call("ec2", "describe-volumes", "--filters", tag)["Volumes"]
    for volume in volumes:
        if ({"Key": "SathonateRun", "Value": RUN} not in volume.get("Tags", [])
                or volume.get("State") != "available" or volume.get("Attachments")
                or not re.fullmatch(r"vol-[a-f0-9]+", volume.get("VolumeId", ""))):
            raise RuntimeError("Refusing to delete an attached or unverified test volume")
    for volume in volumes:
        call("ec2", "delete-volume", "--volume-id", volume["VolumeId"])
    deadline = time.monotonic() + 120
    while call("ec2", "describe-volumes", "--filters", tag)["Volumes"]:
        if time.monotonic() >= deadline:
            raise TimeoutError("Detached test volume deletion remains unverified")
        time.sleep(5)


def cleanup_storage(call, kube, tenant, network, *, clock=time.monotonic, pause=time.sleep):
    project = "token-bound-project"
    raw = json.dumps([tenant, project, network], separators=(",", ":")).encode()
    namespace = "smp-s-" + hashlib.sha256(raw).hexdigest()[:32]
    metadata = kube("get", "namespace", namespace, "--ignore-not-found", "-o", "json").get("metadata")
    volume_ids = []
    if metadata:
        expected = {"tenant": tenant, "project": project, "network": network,
                    "profile": "smp-secure-local-v1"}
        if not metadata.get("uid") or any(
            metadata.get("labels", {}).get(f"fabric.sathonate.io/{key}") != value
            for key, value in expected.items()
        ):
            raise RuntimeError("Refusing cleanup of a namespace with different ownership")
        for pv in kube("get", "pv", "-o", "json")["items"]:
            spec = pv["spec"]
            if spec.get("claimRef", {}).get("namespace") != namespace:
                continue
            csi = spec.get("csi", {})
            volume_id = csi.get("volumeHandle", "")
            if (spec.get("storageClassName") != "smp-gp3-encrypted"
                    or spec.get("persistentVolumeReclaimPolicy") != "Delete"
                    or csi.get("driver") != "ebs.csi.aws.com"
                    or not re.fullmatch(r"vol-[a-f0-9]+", volume_id)):
                raise RuntimeError("Unexpected storage in the test namespace")
            volume_ids.append(volume_id)
        kube("delete", "--raw", f"/api/v1/namespaces/{namespace}", "-f", "-",
             body={"apiVersion": "v1", "kind": "DeleteOptions",
                   "preconditions": {"uid": metadata["uid"]}})
    deadline = clock() + 600
    tagged = bound = []
    while clock() < deadline:
        current = kube("get", "namespace", namespace, "--ignore-not-found", "-o", "json")
        filters = ["Name=volume-id,Values=" + ",".join(volume_ids)] if volume_ids else []
        tagged = call("ec2", "describe-volumes", "--filters",
                      f"Name=tag:SathonateRun,Values={RUN}")["Volumes"]
        bound = call("ec2", "describe-volumes", "--filters", *filters)["Volumes"] if filters else []
        if not current and not tagged and not bound:
            return
        pause(5)
    if any(item["VolumeId"] not in {volume["VolumeId"] for volume in tagged} for item in bound):
        raise RuntimeError("Unverified ownership of a volume still bound to this namespace")
    raise StorageTimeout("PVC/EBS cleanup timed out")


def cleanup(call, make_kube, delete_stack, admin_cleanup, tenant, network, issuer_file, *, fallback=False):
    who = call("sts", "get-caller-identity")
    if who.get("Account") != ACCOUNT or not re.fullmatch(
        rf"arn:aws:sts::{ACCOUNT}:assumed-role/AWSReservedSSO_Sprint3-permissions_[a-fA-F0-9]{{16}}/sathonate-sprint3",
        who.get("Arn", ""),
    ):
        raise RuntimeError("Unexpected cleanup identity")
    result = call("cloudformation", "describe-stacks", "--stack-name", RUN, missing_stack=True)
    issuer = issuer_file.read_text().strip() if issuer_file.exists() else ""
    if issuer and not re.fullmatch(r"https://oidc\.eks\.ap-south-1\.amazonaws\.com/id/[a-fA-F0-9]+", issuer):
        raise RuntimeError("Invalid recorded run issuer")
    if result:
        stack, = result["Stacks"]
        if not re.fullmatch(
            rf"arn:aws:cloudformation:{REGION}:{ACCOUNT}:stack/{RUN}/[a-fA-F0-9-]+", stack["StackId"]
        ) or {"Key": "SathonateRun", "Value": RUN} not in stack.get("Tags", []):
            raise RuntimeError("Unexpected cleanup stack ownership")
        outputs = {entry["OutputKey"]: entry["OutputValue"] for entry in stack.get("Outputs", [])}
        if stack["StackStatus"] != "DELETE_IN_PROGRESS" and any(
            p["ParameterKey"] == "EnableWorkloads" and p.get("ParameterValue") == "true"
            for p in stack.get("Parameters", [])
        ):
            cluster = call("eks", "describe-cluster", "--name", RUN)["cluster"]
            live_issuer = cluster["identity"]["oidc"]["issuer"]
            if (cluster["arn"] != f"arn:aws:eks:{REGION}:{ACCOUNT}:cluster/{RUN}"
                    or cluster.get("tags", {}).get("SathonateRun") != RUN
                    or live_issuer != outputs.get("OIDCIssuer")
                    or (issuer and issuer != live_issuer)):
                raise RuntimeError("Cluster differs from recorded stack ownership")
            issuer = live_issuer
            issuer_file.write_text(issuer)
            try:
                with make_kube(cluster) as kube:
                    cleanup_storage(call, kube, tenant, network)
            except StorageTimeout:
                if not fallback:
                    raise
                print("PVC cleanup deadline reached. Deleting the owned stack before detached volume cleanup.", flush=True)
        elif not issuer and outputs.get("OIDCIssuer"):
            issuer = outputs["OIDCIssuer"]
            issuer_file.write_text(issuer)
    if fallback:
        delete_stack(after_stack=lambda: remove_detached_run_volumes(call))
    else:
        delete_stack()
    admin_cleanup(issuer)


@contextmanager
def kube_client(cluster):
    endpoint = cluster["endpoint"]
    ca = cluster["certificateAuthority"]["data"]
    if not re.fullmatch(r"https://[a-zA-Z0-9.-]+\.ap-south-1\.eks\.amazonaws\.com", endpoint) or not ca:
        raise RuntimeError("Unexpected Kubernetes endpoint or CA")
    config = {
        "apiVersion": "v1", "kind": "Config", "current-context": RUN,
        "clusters": [{"name": RUN, "cluster": {"server": endpoint, "certificate-authority-data": ca}}],
        "contexts": [{"name": RUN, "context": {"cluster": RUN, "user": RUN}}],
        "users": [{"name": RUN, "user": {"exec": {
            "apiVersion": "client.authentication.k8s.io/v1beta1", "command": "aws",
            "args": ["--profile", PROFILE, "--region", REGION, "eks", "get-token", "--cluster-name", RUN],
            "interactiveMode": "Never",
        }}}],
    }
    with tempfile.TemporaryDirectory(prefix="sathonate-sprint3-") as directory:
        path = Path(directory) / "kubeconfig.json"
        path.write_text(json.dumps(config))

        def kube(*args, body=None):
            proc = subprocess.run(
                ["kubectl", "--kubeconfig", str(path), "--context", RUN, "--request-timeout=20s", *args],
                input=json.dumps(body) if body else None, text=True, capture_output=True, timeout=30,
            )
            if proc.returncode:
                raise RuntimeError(f"Kubernetes cleanup failed: {proc.stderr.strip()}")
            return json.loads(proc.stdout) if proc.stdout.strip() else {}

        yield kube


def cleanup_admin(issuer):
    script = Path(__file__).parent / "iam" / "admin-bootstrap.ps1"
    args = ["powershell.exe", "-NoProfile", "-NonInteractive", "-File",
            str(script), "-AdminProfile", "sathonate-bootstrap", "-Action", "Cleanup"]
    if issuer:
        args.extend(["-Issuer", issuer])
    subprocess.run(args, check=True, timeout=300)


def wait_for_cleanup(deadline, signal, *, now=lambda: datetime.now(timezone.utc),
                     pause=time.sleep):
    """Allow completion to trigger the same independent deadline process."""
    while not signal.exists():
        remaining = (deadline - now()).total_seconds()
        if remaining <= 0:
            break
        pause(min(1, remaining))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tenant", required=True, help="exact saved storage scope")
    parser.add_argument("--network", required=True, help="exact saved network ID")
    parser.add_argument("--issuer-file", required=True, type=Path,
                        help="run-specific local journal retained across retries")
    parser.add_argument("--runner-kubeconfig", required=True, type=Path,
                        help="exact kubeconfig path reserved for the secure EKS runner")
    parser.add_argument("--cleanup-signal", required=True, type=Path,
                        help="run-local file to create after recording PASS or FAIL")
    parser.add_argument("--at", required=True, type=datetime.fromisoformat,
                        help="cleanup UTC deadline, or a past UTC timestamp to clean immediately")
    args = parser.parse_args()
    if (not re.fullmatch(r"lp-[a-f0-9]{48}", args.tenant)
            or not re.fullmatch(r"[a-f0-9]{32}", args.network) or args.at.utcoffset() is None):
        parser.error("exact qualification binding and timezone-aware deadline required")
    seconds = (args.at - datetime.now(timezone.utc)).total_seconds()
    if seconds > 4 * 3600:
        parser.error("deadline exceeds qualification window")
    fence = Path(str(args.runner_kubeconfig.resolve()) + ".cleanup-fence")
    wait_for_cleanup(args.at, args.cleanup_signal)
    fence.touch(exist_ok=True)
    cleanup(aws, kube_client, teardown, cleanup_admin, args.tenant, args.network, args.issuer_file, fallback=True)
