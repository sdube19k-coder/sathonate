# Sprint 3 quick EKS qualification

Use the [manual setup and AWS runbook](manual-setup-and-aws-runbook.md) for
workstation, Okta, PostgreSQL, Identity Center and operator commands.

The local console and API stay in History mode until the operator has explicit
approval for the paid run. Use the browser for the customer deployment. The
operator owns AWS setup and teardown. The approved ceiling is USD 4.

## Before starting the clock

- Recheck the disposable project's expiring Okta grant and use **Verify
  deployment access** on the saved design. Do not grant access on the retained
  project. Record the exact saved network ID, storage scope and typed design hash.
- After authorization for AWS access, refresh both SSO profiles. Check the
  account and assumed-role identities, available quota, standard-support
  version, exact cluster/stack absence, and the repository ownership tag. Confirm
  both sessions cover the launch deadline and cleanup reserve. Verify the effective
  permissions for issuer binding, stack deletion, ECR removal, and run IAM cleanup.
- Generate the administrator-owned template with
  `python tools/sprint3/prepare_admin_owned.py --output <new-directory>`.
  Validate it, check the current cost estimate against the ceiling, then obtain
  a fresh **Yes** before starting anything billable, including image storage.
- Prepare a deadline and an operator who can issue cleanup from a separate
  terminal if the browser or backend exits. Budget for teardown before the
  deadline. The stack helper bounds polling, not the AWS bill.
- Reserve the exact runner kubeconfig path before launch. Arm
  `tools/sprint3/cleanup_run.py --tenant <storage-scope> --network <saved-id>
  --issuer-file <run-journal> --runner-kubeconfig <absolute-runner-kubeconfig>
  --cleanup-signal <run-local-signal> --at <UTC-deadline>` in a separate
  terminal. Use a new signal path for each attempt. Create the signal file as
  soon as the result is recorded to trigger the same process immediately.
  Preserve the journal on retries.
  Its adjacent cleanup fence cancels the secure EKS runner and prevents new
  submissions or re-adoption. Never remove that fence to retry a deployment.

## Run and capture

1. Publish only the qualified images to the run repository. Record returned
   digests and require the controller manifest digest to match local qualification.
2. Create the exact run stack with workloads disabled. Record its immutable
   stack ID, VPC and OIDC issuer. Bind the issuer using the bootstrap role, then
   enable the three node groups and add-ons. Install the approved controller
   CRDs and [encrypted gp3 StorageClass](../tools/sprint3/storage-class.yaml).
3. Write the scoped kubeconfig and assemble the runner binding using the actual
   issued cluster CA, endpoint and image digests. Follow the
   [secure EKS adapter configuration](sprint3-local-secure-eks.md).
4. In the authorized Sathonate project, open **Sprint 3 protected EKS
   qualification**, deploy the saved revision, and record deployment progress,
   health, and the committed transaction with peer readback. Record failures
   and begin cleanup immediately if the deadline is reached.

## Cleanup on success, failure, or timeout

1. Stop submissions and fence the run's reconciliation. Verify the disposable
   cluster and namespace identity. Record the owned PVC UIDs and backing volume
   IDs. Remove the run workloads and PVCs while CSI is still active. Confirm
   those volumes are deleted before calling stack teardown. Never use the local
   retained Kubernetes context for any of these actions.
2. Run `python tools/sprint3/teardown_stack.py --workloads-cleaned`. It deletes
   only the owned stack and checks regional EKS, EC2 and ELB inventory. Inspect
   CloudFormation events and reconcile any reported leftovers by verified
   ownership. Separately verify backing volume IDs and the exact log group.
   The independent cleanup helper can instead run immediately with a past
   deadline. If the UID-bound namespace delete was accepted but PVCs stall,
   it allows stack deletion and removes only detached EBS volumes with the
   exact run tag. Unexpected ownership fails closed.
3. Empty and delete only the run ECR repository using the scoped deployer role
   and verify it is absent. Run the administrator cleanup helper with the
   recorded issuer to remove the run roles and provider. Verify their absence
   before revoking the temporary Identity Center assignment. Shared service-linked
   roles and unrelated KMS keys are outside the run.
4. Record cleanup evidence with the test result. Escalate any failed cleanup
   immediately, even if the Fabric test passed.

The local rehearsal uses fixtures. Live issuer binding, image publication,
runner startup, PVC deletion and complete cleanup require live evidence.
