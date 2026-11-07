# Local retained-data deletion and cold recovery

Bounded local acceptance passed; see the [Sprint 5 report](sprint5-acceptance.md) and [final receipts](evidence/sprint5-final/summary.json). This operating guide does not extend that tested scope. These operations are restricted to the protected, configuration-driven local profile in a disposable kind target. AWS and production remain disabled.

## Retained-data workload deletion

An administrator with server-owned cleanup permission reviews `GET /api/v1/deployments/{latest-operation}/cleanup-preview`, then explicitly approves its exact digest through `DELETE /api/v1/deployments/{latest-operation}` with `X-Sathonate-Cleanup-Approval`. The customer-configuration console exposes the same review, inventory and explicit checkbox.

The only supported policy removes owned Deployments, worker Jobs and the controller/issuer execution permissions. The controller is removed first, every DELETE tests the original UID, and a namespace retirement fence blocks automatic redeployment. The durable journal binds actor, target, namespace, actions and retained UIDs before mutation; a retry continues that exact intent, while acknowledgement does not repeat deletion. Foreign workloads, replaced objects, active operations, missing ownership and stale previews fail closed.

Namespace, Fabric CRs, Secrets, Helm history, PVCs, services, configuration and containment policies remain. The implementation checks retained UIDs and refuses garbage-collection ownership links to deleted objects. It deliberately does not delete Fabric CRs: the current operator's finalizers invoke Helm uninstall, which can destroy PVCs. This is workload removal with retained data, NOT complete network erasure, cloud teardown or secure destruction. Restarting a retired network requires a separate qualified procedure; clearing the fence is not a supported customer operation.

## Offline encrypted cold capture and restore

`cmd/fabric-cold-backup` is a local administrator utility. It requires explicit `SATHONATE_APP_ENV=local-development`, protected target configuration, a separate protected 32-byte key file and the PostgreSQL connection in the process environment. Never put passwords, connection strings or key bytes in arguments. This qualification utility accepts only the loopback `fabric_smp_tests` database, rejecting other hosts/databases before connection. The API/executor must be stopped: the utility acquires its PostgreSQL advisory lock and rejects active operations and superseded inputs.

```powershell
# Load DATABASE_URL securely from the approved local credential provider.
$env:SATHONATE_APP_ENV='local-development'
./bin/fabric-cold-backup.exe -action capture -secure-local-config <private-target.json> -tenant <verified-storage-scope> -operation <latest-verified-operation> -key-file <private-key.bin> -artifact <new-encrypted-artifact>
# Save the public binding object from the capture receipt separately.
./bin/fabric-cold-backup.exe -action restore -secure-local-config <same-private-target.json> -tenant <same-scope> -operation <same-operation> -key-file <same-private-key.bin> -artifact <encrypted-artifact> -binding <capture-binding.json>
./bin/fabric-cold-backup.exe -action resume -secure-local-config <same-private-target.json> -tenant <same-scope> -operation <same-operation> -key-file <same-private-key.bin> -artifact <encrypted-artifact>
```

Capture binds original namespace, Deployment/template and PVC UIDs, fences the namespace, stops the controller before all Fabric and chaincode writers, verifies that no live/pending pod remains, and reads each original bound volume through a tokenless helper using the trusted worker image. The helper has only the selected PVC; capture/verification mounts are read-only. Explicit file-ownership capabilities are confined to that offline helper for ownership-preserving restoration. No host mounts or new RBAC privileges are introduced.

The AES-256-GCM artifact contains compressed CA/peer/orderer files, captured identity/configuration resources and a repeatable-read snapshot of only that tenant/network's drafts, revisions, operations and deployment records. Plaintext payloads remain in memory and private pipes. The bounded format is limited to 16 MiB for the complete plaintext bundle, 128 MiB expanded per volume and 100,000 archive members; larger datasets need an approved storage snapshot system. Artifacts use no-overwrite private file creation; key custody remains separate.

Restore authenticates the whole artifact before mutation. It refuses another target, tenant, operation, namespace or replacement PVC, unsafe archives, nonempty original volumes and divergent existing records/material. Missing captured Secrets may be recreated with their exact captured bytes under the fenced original namespace; restored identity provenance and contents are checked on retry. Missing rows for only the backed-up network may be inserted atomically; existing divergent rows are never rolled back. No memberships, permission grants, database roles or schema are imported. Per-volume acknowledgement markers require exact file/ownership verification before retry can adopt them.

Resume verifies restored/original file inventory, contents and ownership, starts the original Deployments, brings the controller back last, and clears the cold fence only after rollout readiness. Keep the API stopped until resume completes, then perform authenticated fresh committed transactions and ledger checks. An interrupted or refused operation remains fenced. If a transfer fails before its acknowledgement, do not overwrite nonempty storage or clear the fence; preserve the private artifact and inspect the checkpoint. Automatic recovery from every partial snapshot/restore interruption is not yet qualified.

## Acceptance boundaries

Live tests must distinguish original-PVC cold recovery, exact missing identity/control-row restoration, retained-data workload removal and production disaster recovery. The first three can be tested in fresh disposable networks; none establishes replacement-cluster/PVC recovery, PostgreSQL server/schema/role recovery, multi-server storage recovery, historical exposed-key remediation, real Okta acceptance or production readiness. Retained user networks are never qualification targets.
