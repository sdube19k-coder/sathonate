# Historical key exposure: incident and migration procedure

**BLOCKED pending owner approval and incident disposition.** This plan is not remediation evidence. Original controller and retained 8/8 network remain unchanged. Do not read/export raw exposed keys, erase evidence, automatically rotate or adopt the candidate into production.

## Exposure and containment

The pinned original orderer controller logs signing/TLS private-key variables. Prior retained audit confirmed private-key markers without exporting key bodies. Treat logs, aggregators, diagnostics, audit payloads, caches and backups as potentially credential-bearing. Historical readers may impersonate affected identities until appropriate trust is replaced/revoked. Deleting logs does not undo exposure. Existing cluster-wide controller privileges increase blast radius. The candidate's removed statement does not remediate history.

Source: pinned original `controllers/ordnode/ordnode_controller.go:1258` at `cd236ccc61d1d159b3dadb4625d08bb5682a8451`. The statement prints `signEncodedPK` and `tlsEncodedPK`. Existing redacted evidence `.logs/controller-upstream-log-audit.json` records `privateKeyMarkerObservedInSample=true`, `rawLogsExported=false`, `liveCredentialsRotated=false`. The resumed review read only that boolean audit and public source, not retained raw logs or keys.

An authorized incident owner must restrict log/backup access, preserve audit/access history under restricted immutable retention, determine affected time ranges/principals and downstream exports, and assess notifications with incident/legal owners. Do not collect private keys to investigate. Engineering evidence contains only public fingerprints and metadata. No historical evidence deletion is part of this plan.

## Inventory and preflight

1. Assign incident/approval IDs, accountable owner, maintenance window and exact identities/controllers/organizations/channels authorized for change. Record owner-approved ledger/transaction/quorum baselines; agent readiness alone is insufficient.
2. Inventory affected **public certificates only**: SHA256 DER fingerprint, serial, issuer, role, validity, endpoints. Never export a key for fingerprinting. Include roots when the entire authority is affected. Unknown exposure scope is a blocker, not an empty safe inventory.
3. Install approved public fingerprints in trusted runtime `deniedCertificateSHA256` and `deniedPublicKeySHA256`. Verify fresh identities differ and leaf/root denial tests pass. Requests cannot clear/override it. Retained namespaces cannot be adopted as fresh secure deployments.
4. Use the approved encrypted backup service for Kubernetes/CA/PVC/ledger/configuration snapshots, strict key separation and a tested isolated restore. Record hashes/results only. Preserve historical Helm/log evidence under restricted retention. Qualify restore before any retained change.

## Migration requiring separate execution approval

1. Reproduce exact topology independently with approved immutable candidate/issuer images, restricted RBAC and enforcing CNI/admission. Validate CRD/chart/controller pins; do not invent upstream fields or call legacy password-bearing Helm state secure.
2. Qualify each identity separately: peer TLS, signing/MSP, operations TLS, orderer admin TLS, Raft consenter TLS, channel admin/client, registrar, CA signing/root. Six peer-TLS cases cover only peer TLS. Other classes need issuance, revocation/expiry, trust overlap, interruption and restore/forward-recovery evidence before scheduling.
3. Coordinate channel MSP/root/admin policies and consenter changes across authorized organizations. Preserve Raft quorum and verify every channel. Kubernetes rollout cannot update channel consensus identities. Do not rotate all consenters together without a separately proven procedure.
4. Pre-enroll fresh identities in validated immutable bundles. Validate all required material before switching one template/reference. Use checkpoints/idempotent retry and an approved retention policy. No credentials in public CRs, ConfigMaps, Helm, argv, diagnostics or evidence.
5. Transfer reconciliation ownership in a planned window with only one active writer per workload. Stop/contain the legacy controller only when approved. Verify references, mounts, live certificates, both-peer committed/query transactions, ledger continuity and quorum before completion acknowledgement.
6. Revoke/remove exposed identities/trust after overlap/recovery gates pass. Prove stale certificates cannot authenticate, endorse or administer. Assess copied credentials, offline backups and historical readers. Obtain incident closure, independent security sign-off and explicit production authorization.

## Rollback boundaries

Before channel/MSP/consenter changes, a workload may select a retained **unexposed** valid bundle/successful Helm release. Compromised identities are not a safe long-term fallback. Use fresh approved recovery identities where exposure excludes old material. Preserve ledger/PVC/CA databases; never delete persistent state to force success.

After channel trust/Raft changes, Helm rollback is insufficient and can break authentication/quorum. Use the separately qualified coordinated Fabric forward-recovery procedure. Stop at unsupported boundaries and escalate to the owner. No automated signing/CA/Raft mutation is enabled here. Missing restore/recovery qualification blocks production.

Acceptance must record approval IDs, public fingerprints, source/image hashes, transitions/checkpoints, live certificates, both-peer ledger/transaction evidence, quorum, stale-identity denial, secret-free inspections, restore and retention decisions. Private material stays in approved secret/backup systems. Close the retained gate only after authorized execution and independent evidence.

## Public fingerprint calculation

Use an independently approved **public certificate file only**, never an extracted private key or retained log. These commands avoid binary shell pipelines:

```powershell
openssl x509 -in approved-public-cert.pem -outform DER -out approved-public-cert.der
openssl dgst -sha256 approved-public-cert.der
openssl x509 -in approved-public-cert.pem -pubkey -noout -out approved-public-key.pem
openssl pkey -pubin -in approved-public-key.pem -outform DER -out approved-public-key.der
openssl dgst -sha256 approved-public-key.der
```

The first digest belongs in `deniedCertificateSHA256`; the second belongs in `deniedPublicKeySHA256`. Record leaf and affected root identifiers. Public-key denial prevents the same exposed key being accepted under a newly issued certificate. No private-key access is needed. Unknown exposure scope still blocks incident closure.
