# Backup, retention and cleanup

## Ownership and inventory

The approved network/tenant/target owns its exclusive namespace, Fabric CRs, enrollment and wallet Secrets, CA database/ledger PVCs, worker Jobs/ConfigMaps/RBAC and any externally created backups or cloud volumes. The cleanup preview inventories exact deletion objects and retained Secret/PVC **names and UIDs only**, without data or annotations. Cloud EBS/snapshot discovery and storage-layer retention are explicitly not verified; no AWS inventory is silently performed. Operator-owned Helm release Secrets/history may also contain enrollment values. Retention must cover those stores and audit/log backups, not only the primary Secret.

Record owner, purpose, approved minimum/maximum retention, legal/business constraints, recovery point/time, encryption-key custodian, backup access, expiry review and destruction authority in the organization's private change system. No default duration is presented as a universal legal policy. Tenant application users must not read CR/Helm/Secret payloads or act as backup administrators.

## Local encrypted bundle tool

`fabric-maintenance` is a local AES-256-GCM file-bundle encryption/restore tool, not an automatic Kubernetes/ledger snapshotter. It authenticates schema, exact tenant/network/target/spec binding and capture time, uses fresh random nonces, refuses malformed/tampered/wrong-key/wrong-binding input, and refuses output overwrite. Restore restricts the output file before writing plaintext (current-user DACL on Windows; 0600 on Unix). Receipts contain only binding/ciphertext hashes and timestamps. Keep key files in an independently controlled secret facility; the CLI consumes a protected **32-byte binary key file**, never an inline key or connection string. It does not attest the supplied key file's provenance or security.

```powershell
Set-Location E:\Sathonate\fabric-control-plane
go build -o bin/fabric-maintenance.exe ./cmd/fabric-maintenance
./bin/fabric-maintenance.exe -action backup -binding <private-binding.json> -key-file <protected-32-byte-key-file> -input <authorized-frozen-bundle> -output <new-encrypted-backup>
./bin/fabric-maintenance.exe -action restore -binding <same-private-binding.json> -key-file <protected-32-byte-key-file> -input <encrypted-backup> -output <new-private-recovery-file>
./bin/fabric-maintenance.exe -action inspect-certificate -input <public-certificate.pem>
```

Binding JSON contains only `tenantId`, `networkId`, `targetSHA256`, `networkSHA256`. Certificate inspection refuses private keys and reports fingerprint/validity times, never claims trust-chain verification. Input bundles are limited to 16 MiB; larger/streaming volumes require an approved snapshot tool. The encryption key is not embedded in the backup. Losing it prevents recovery. Restoration tests with synthetic secret payloads passed locally, including tamper/wrong-tenant/wrong-key refusal and output ACL/no-overwrite behavior. These tests do not establish recovery of a running Fabric network.

## Consistent backup and restoration (not yet qualified)

Before real rotation/destruction: stop or quiesce the appropriate writers through an approved maintenance window, preserve CA databases and identity mappings, take consistent ledger/PVC snapshots, back up the control-plane PostgreSQL schema/data and approved target/operation metadata, and capture private key/CR/Helm references securely. Use restricted service/credential files for database backup tooling, not DSNs or passwords in command arguments. Do not put Secret exports into a normal report directory. Encrypt the selected frozen bundle and retain its key separately. A live backup/export driver is not implemented by this command.

Restore first into a disposable environment with exact matching target/binding, compatible images and isolated identities. Verify backup integrity, database consistency, CA enrollment/registry state, peer ledger hashes/heights, three active consenters, channel membership, signing/TLS keys/certificates and a committed write/query from both organizations before claiming recovery. Reusing restored identities in two active environments can violate security assumptions. No cloud snapshot, PVC restore or live CA restore has been qualified here.

## Cloud workload cleanup preview and approval

These commands require a running, separately approved loopback EKS qualification API. Do not run them against the retained user network or AWS without explicit authorization. Obtain a fresh metadata preview first:

```powershell
$base='http://127.0.0.1:8082/api/v1'
$preview=Invoke-RestMethod "$base/deployments/<operation-id>/cleanup-preview"
# Review exact names/UIDs, binding hashes, retained objects and cascading risks.
# Only after explicit administrator approval:
Invoke-RestMethod "$base/deployments/<operation-id>" -Method Delete -Headers @{'X-Sathonate-Cleanup-Approval'=$preview.digest}
```

The API requires administrator role/write scope; wrong tenant/target/network, an active deployment, stale preview, replaced UID or unowned resource fails closed. New workloads carry namespace ownership labels. Old unlabelled cloud resources require a separately reviewed ownership migration; approval does not bypass missing ownership. The controller issues exact named DELETEs with UID preconditions, observes removal and revokes the exact worker access. It does not delete namespaces, Secrets, PVCs, cloud volumes or external snapshots. Operator finalizers/cascades may delete their own subordinate resources: retained-object metadata and actual storage behavior must be compared separately, and `retentionVerified` remains false.

After interruption, preview remaining objects again, investigate partial effects and explicitly approve the new digest. Do not clear finalizers, broaden selectors, delete immutable Secrets or force-delete PVCs to make a cleanup test pass. Local development namespace cleanup is preserved only for explicitly owned local networks; retained user data is never a test target.

## Destruction limits

There is deliberately no generic tenant-facing Secret/PVC/cloud-volume destruction endpoint. Removing a workload or password is not secure erasure. EBS reclaim policies, operator-owned Secrets, Helm history, snapshots, audit/log archives, host caches, tmpfs/crash remnants and external backups need separate verification and owner-approved destruction. Encryption-key retirement can affect multiple backups and is not automatically authorized by workload cleanup. No cloud destruction was executed.
