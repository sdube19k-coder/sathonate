# Sprint 4 — local operations and troubleshooting

Historical increment report. For the 5-6 November dynamic MVP result, see [current acceptance](sprint4-dynamic-acceptance.md). Original dates and results below remain unchanged.


Scope selected on 30 October 2026 after reviewing the existing API, console,
architecture and qualification tools. No numbered Sprint 4 backlog existed in
the repository. Existing network design/project authorization, provisioning,
operation history, retry, channel/chaincode deployment and health/log views are
reused rather than rebuilt.

## Implemented scope

- Authenticated project-scoped `GET /api/v1/operations/{id}/diagnostics`.
  The same server-resolved tenant/project storage scope protects operation
  history and this endpoint. Foreign objects return 404; unauthenticated requests
  are rejected. Read-only memberships suffice and revocation applies on every
  request, including history-only mode.
- An explicit diagnostic payload allowlist: operation/network IDs, status,
  lifecycle timestamps, retry reference, cancellation flag, stage status/timing,
  event count and fixed troubleshooting guidance. It excludes actors,
  idempotency keys, desired designs, event/error text, logs, Kubernetes payloads
  and all credential stores. Responses are not cached.
- Console operation history downloads the report through the authenticated API.
  Loading/error states and an accessible per-operation button are included.
  Reports identify recorded evidence and never claim to probe current health.
- Disposable API acceptance now restarts a peer, verifies that the pod changed
  while PVC UIDs did not, reconciles through the API, and checks a second new
  transaction and advancing ledger heights on both peers. This is pod/PVC reuse,
  not backup restoration or survival of node/disk loss.
- Browser qualification now requires the freshly built API's explicit SHA256
  and preserves the qualified worker image identity instead of requiring an
  obsolete Sprint 1 source commit. It exercises the diagnostic download and
  foreign-project rejection against real local workloads.

## Boundaries and remaining backlog

The supported execution topology remains the qualified two-organization,
two-peer, two-CA, three-orderer profile. Arbitrary topology edits, adding/removing
organizations/peers/orderers and general channel/chaincode management need
separate guarded workers, change plans, consensus/membership validation and
recovery acceptance. Editing a saved design does not apply it to a live network.
These broader lifecycle features are not declared complete by the diagnostic
feature or a successful fixed-profile test.

Customer self-service account/tenant creation is not enabled. Server-owned
project memberships remain administrator approved. Real Okta login and the
historical exposed-key remediation remain mandatory security gates. No new
privilege grants, policy changes or retained-network rotations are authorized.

Only Docker-hosted kind has been qualified. Native multi-server Kubernetes,
independent-host failures, production storage/backup restore and AWS EKS are
not proven. Local Calico checks do not qualify AWS VPC CNI or IRSA/EBS.

Sprint 3 stays open: AWS EKS qualification is deferred. Preserve the secure EKS
adapter, IAM/ECR preparation and earlier atomic evidence. AWS provisioning,
permission changes and new infrastructure costs remain prohibited.

## Current acceptance — 30 October 2026

**Local qualification PASS. Sprint 4 local operations increment PASS; broader
feature backlog remains open. Production release BLOCKED.** No AWS calls,
infrastructure creation or IAM changes were performed.

| Check | Current result |
| --- | --- |
| Retained kind node, system workloads and active Fabric network | PASS; node Ready, active deployments 8/8, seven PVCs Bound; read-only inspection |
| Two organizations, two CAs, two peers, three active Raft consenters | PASS, fresh disposable API-controlled network |
| Channel create/join, chaincode install/approve/commit/Put/Get | PASS; `smpchannel`, `smp_smoke` 1.0 sequence 1 |
| API authentication, idempotency, application restart/adoption and failed-worker retry | PASS using controlled HTTPS OIDC; not real Okta |
| Peer replacement and persistent storage | PASS; changed pod UID, unchanged PVC UIDs, new committed transaction, both ledgers advanced to height 6 with equal block hash |
| TLS, namespace/RBAC/foreign-resource protection, effective Calico ingress/egress and admission | PASS; fourteen runtime inspection checks |
| Runtime public CR/ConfigMap, Helm history, sampled argv, logs and source/evidence credential inspection | PASS; twelve Helm history records and eleven log streams in API run; independent UI inspection also passed |
| Console → authenticated API → real Fabric, refresh, interruption/retry, diagnostic download, isolation and production auth denials | PASS; nineteen browser checks, no browser runtime errors |
| Go/vet | PASS; 253 tests/subtests, 16 explicit opt-in skips; skips do not count as runtime acceptance |
| PostgreSQL authorization/RLS/recovery | PASS; 40 tests/subtests, one TLS opt-in skip; separate fresh verified-TLS/restricted-role Docker fixture PASS |
| Console units/build/format | PASS; 18 tests, build and formatting |
| Offline infrastructure regression subset | PASS; five tests, no AWS calls |
| Independent multi-server Kubernetes / disk-loss or backup restore | NOT TESTED |
| Independent native engine / arbitrary live topology changes | NOT TESTED |
| Live EKS, IRSA, AWS CNI/EBS and cloud teardown | NOT TESTED; deferred |
| Real Okta / historical key-exposure remediation | BLOCKED; unchanged owner/external security gates |

The retained cluster initially showed transient startup readiness after the host
restart and recovered without mutation. One historical failed worker pod remains
outside the active retained-network namespace: `smp-n-4a3ac39ceab00d34` (13 days
old). It was preserved. The first qualification attempt stopped before cluster
creation because the existing development PostgreSQL container was stopped.
Starting only `smp-postgres` resolved that local dependency. Its failed-attempt
receipt is preserved; no failed runtime check is relabeled PASS.

New API-run transactions:

- Initial committed write/read: `3ed04381a8741a07f8eaf9fb9230ab6cfd2914f47be6ecf3ae1c55f1be18bc94`.
- After peer replacement: `1c35a2f7def1f99a5e10ac3a5e66cde8f8d1587bacc27795567d767112559ae3`.
- Fresh browser-submitted deployment: `1cb9d8b127be9c79664006a72e6e18521d4eedef9bbf66b45dcfb3b9de1c6fcb`.

The protected worker submits a fresh timestamp-keyed `Put`, then calls `Get` on
both organizations and fails unless each value equals `ready-<namespace>`.
For the API run that expected value was
`ready-smp-s-ce32fc5e150cff9526aca7b6d0bb1943`. The key is not retained in its
public result. This is actual worker verification, not a separate manual CLI
demonstration. Source: `internal/kubernetesfabric/worker.go`, strict transaction
ID/value validation and peer ledger decoding. No new transaction was submitted
to the retained network.

Committed redacted manifest: [evidence/sprint4-local-verification.json](evidence/sprint4-local-verification.json).
It includes source hashes, image/binary identities, new transaction IDs, ledger
hashes, test totals, timestamps, limits and evidence hashes. Raw redacted command
outputs are in backend `.logs/local-final-20261030/` and console
`.logs/local-final-20261030/`. Earlier atomic evidence is preserved unchanged.

Both disposable kind clusters and temporary TLS database were removed. Only
`smp-qualification` remains in `kind get clusters`; retained PostgreSQL and the
retained kind container remain intentionally. Test API/Vite listeners on
8080/5173/5174 stopped, and private fixture grant/config files were removed.

## Commands used

From the backend repository, Docker connectivity and retained metadata were
read with `docker info --format '{{.ServerVersion}}'` and explicit retained
`kubectl --kubeconfig ... --context kind-smp-qualification get` commands.
The current runtime command was:

```powershell
$env:PATH='E:/Sathonate/tools/kubectl-1.34;'+$env:PATH
$env:SMP_QUALIFICATION_PREFIX='sprint3'
$env:SMP_QUALIFICATION_WORKER='sathonate/fabric-worker:sprint3'
$env:SMP_QUALIFICATION_CALICO='1'
$env:SMP_SECURE_OIDC_FIXTURE='1'
python tools/sprint1/qualify.py
python tools/sprint2/postgres.py --output .logs/local-final-20261030/postgres.jsonl
```

The historical `sprint3` output prefix does not imply AWS execution. The new
runtime receipts were copied to the dated evidence directory. Fresh Go tests,
vet and API build used the existing offline Go cache:

```powershell
go test ./... -count=1 -json
go vet ./...
go build -o bin/fabric-api-sprint4.exe ./cmd/fabric-api
$env:SMP_POSTGRES_TLS_E2E='1'
go test ./internal/persistence -run '^TestProductionPostgresTLSAndRoleOnDocker$' -count=1 -v
```

From the console:

```powershell
$env:SMP_UI_API_SHA256=(Get-FileHash E:/Sathonate/fabric-control-plane/bin/fabric-api-sprint4.exe -Algorithm SHA256).Hash.ToLowerInvariant()
python tools/qualify-ui.py
```

The harness receives generated tokens over stdin and hashes in protected local
grant files; no bearer token belongs in these commands or report files.

## Next local work

1. Build guarded change planning and recovery for peer/organization/orderer
   membership changes before enabling those customer controls. Preserve the
   qualified fixed profile until those new transitions pass acceptance.
2. Extend channel/chaincode management beyond the fixed qualification contract,
   with server-owned authorization, idempotent operations and rollback limits.
3. Complete owner-assisted real Okta enrollment/callback/API/logout acceptance
   and historical exposure inventory/rotation/restore approvals. Do not rotate
   retained identities automatically or infer user membership from groups.
4. Qualify multi-server placement, storage-loss recovery and backups on an
   explicitly supplied test environment. Keep AWS deferred and production off.

Pre-existing uncommitted AWS budget-preparation drafts were left untouched and
excluded from the verified Sprint 4 commits.
