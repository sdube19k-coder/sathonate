"""Delete only the verified Sprint 3 stack after workload and PVC cleanup."""
import argparse
import json
import re
import subprocess
import time

ACCOUNT = "429488690613"
REGION = "ap-south-1"
RUN = "sathonate-s3-20261027"
PROFILE = "sathonate-sprint3"


def aws(*args, missing_stack=False):
    result = subprocess.run(
        ["aws", *args, "--profile", PROFILE, "--region", REGION,
         "--output", "json", "--no-cli-pager", "--cli-connect-timeout", "5",
         "--cli-read-timeout", "15"],
        capture_output=True, text=True, timeout=30, check=False,
    )
    if result.returncode:
        if missing_stack and re.search(
            r"\(ValidationError\).*Stack with id .* does not exist", result.stderr
        ):
            return None
        raise RuntimeError(f"{' '.join(args[:2])} failed: {result.stderr.strip()}")
    return json.loads(result.stdout) if result.stdout.strip() else {}


def teardown(call=aws, *, timeout=900, clock=time.monotonic, pause=time.sleep,
             after_stack=lambda: None):
    who = call("sts", "get-caller-identity")
    if who.get("Account") != ACCOUNT or not re.fullmatch(
        rf"arn:aws:sts::{ACCOUNT}:assumed-role/AWSReservedSSO_Sprint3-permissions_[a-fA-F0-9]{{16}}/sathonate-sprint3",
        who.get("Arn", ""),
    ):
        raise RuntimeError("Expected the approved Sprint 3 SSO role and user")
    result = call("cloudformation", "describe-stacks", "--stack-name", RUN, missing_stack=True)
    if result is None:
        after_stack()
        verify_inventory(call)
        return "Run stack absent. Regional test inventory checked."
    stack, = result["Stacks"]
    stack_id = stack["StackId"]
    if not re.fullmatch(
        rf"arn:aws:cloudformation:{REGION}:{ACCOUNT}:stack/{RUN}/[a-fA-F0-9-]+",
        stack_id,
    ) or {"Key": "SathonateRun", "Value": RUN} not in stack.get("Tags", []):
        raise RuntimeError("Stack ownership check failed")
    vpc = next((item["OutputValue"] for item in stack.get("Outputs", [])
                if item["OutputKey"] == "VPC"), None)
    if vpc and not re.fullmatch(r"vpc-[a-f0-9]+", vpc):
        raise RuntimeError("Unexpected VPC output")
    deadline = clock() + timeout
    if stack["StackStatus"] == "UPDATE_IN_PROGRESS":
        call("cloudformation", "cancel-update-stack", "--stack-name", stack_id)
    while stack["StackStatus"] in (
        "UPDATE_IN_PROGRESS", "UPDATE_ROLLBACK_IN_PROGRESS",
        "UPDATE_ROLLBACK_COMPLETE_CLEANUP_IN_PROGRESS",
        "UPDATE_COMPLETE_CLEANUP_IN_PROGRESS", "ROLLBACK_IN_PROGRESS",
    ):
        if clock() >= deadline:
            raise TimeoutError(f"Stack rollback unverified at deadline: {stack_id}")
        pause(min(10, max(0, deadline - clock())))
        result = call("cloudformation", "describe-stacks", "--stack-name", stack_id, missing_stack=True)
        if result is None:
            after_stack()
            verify_inventory(call, vpc)
            return "Run stack absent. Regional test inventory checked."
        stack, = result["Stacks"]
    if stack["StackStatus"] != "DELETE_IN_PROGRESS":
        call("cloudformation", "delete-stack", "--stack-name", stack_id)
    while clock() < deadline:
        result = call("cloudformation", "describe-stacks", "--stack-name", stack_id, missing_stack=True)
        if result is None:
            break
        current, = result["Stacks"]
        if current["StackStatus"] == "DELETE_COMPLETE":
            break
        if current["StackStatus"] == "DELETE_FAILED":
            raise RuntimeError(f"Stack deletion failed: {current.get('StackStatusReason', '')}")
        pause(min(10, max(0, deadline - clock())))
    else:
        raise TimeoutError(f"Stack deletion unverified at deadline: {stack_id}")
    after_stack()
    verify_inventory(call, vpc)
    return "Run stack deleted. Regional test inventory checked."


def verify_inventory(call, vpc=None):
    clusters = call("eks", "list-clusters")["clusters"]
    leftovers = ["EKS cluster"] if RUN in clusters else []
    tag = f"Name=tag:SathonateRun,Values={RUN}"
    for operation, key in (
        ("describe-volumes", "Volumes"),
        ("describe-network-interfaces", "NetworkInterfaces"),
        ("describe-addresses", "Addresses"),
        ("describe-vpcs", "Vpcs"),
    ):
        if call("ec2", operation, "--filters", tag)[key]:
            leftovers.append(key)
    instances = call("ec2", "describe-instances", "--filters", tag,
                     "Name=instance-state-name,Values=pending,running,stopping,stopped,shutting-down")
    if any(reservation["Instances"] for reservation in instances["Reservations"]):
        leftovers.append("EC2 instances")
    gateways = call("ec2", "describe-nat-gateways", "--filter", tag,
                    "Name=state,Values=pending,available,deleting,failed")
    if gateways["NatGateways"]:
        leftovers.append("NAT gateways")
    load_balancers = call("elbv2", "describe-load-balancers")["LoadBalancers"]
    if vpc and any(lb["VpcId"] == vpc for lb in load_balancers):
        leftovers.append("load balancers in the run VPC")
    for lb in load_balancers:
        if vpc and lb["VpcId"] == vpc:
            continue
        tags = call("elbv2", "describe-tags",
                    "--resource-arns", lb["LoadBalancerArn"])["TagDescriptions"]
        if any({"Key": "SathonateRun", "Value": RUN} in entry["Tags"]
               for entry in tags):
            leftovers.append("run-tagged load balancer")
            break
    if leftovers:
        raise RuntimeError("Run resources remain: " + ", ".join(leftovers))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--workloads-cleaned", action="store_true",
                        help="Confirm test PVC cleanup while CSI was active, or no workloads were started")
    args = parser.parse_args()
    if not args.workloads_cleaned:
        parser.error("--workloads-cleaned is required before stack deletion")
    print(teardown())
