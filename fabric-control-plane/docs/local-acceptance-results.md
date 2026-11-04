# Local acceptance results — 2026-10-17

The complete React → Go → PostgreSQL → Kubernetes Job → Bevel operator → Fabric workflow was exercised on this Windows machine. No AWS/Azure provisioning or cloud credentials were used. The working Docker-only implementation was preserved; it was not restarted alongside kind.

## Commands executed and results

| Command (repository directory) | Result |
| --- | --- |
| `docker desktop start --timeout 30`; `docker start smp-postgres smp-qualification-control-plane` | Existing local infrastructure resumed; Docker 29.7.2 and kind Kubernetes 1.34.0 available. |
| `scripts/bootstrap-operator.ps1` (Go) | Ansible/Helm bootstrap passed, including pinned source, dedicated kind provider, matching CRDs and controller readiness. |
| `scripts/setup-local.ps1` (Go) | Full setup rerun passed, including image builds/loading, PostgreSQL readiness and migrations. The corrected bootstrap preserved the healthy network. |
| `go test ./...` (Go) | Passed. Includes API validation/revision/idempotency, tenant isolation, mock failure/retry, persistence failure, complete Fabric evidence checks, secret redaction, profile rejection and package determinism. |
| `go vet ./...` (Go) | Passed. |
| `go test ./internal/persistence ./internal/deployment -count=1`, with `SATHONATE_TEST_DATABASE_URL` pointing privately to `fabric_smp_tests` | Passed against actual PostgreSQL. Draft revision/isolation and deployment state/restart persistence checked. |
| PowerShell parser over `scripts/*.ps1` | Passed. |
| `npm run build`; `npm test`; `npm run format:check` (React) | Passed; 9 unit tests. |
| `npm exec playwright -- test e2e/pdf-flow.spec.ts -g 'PDF cloud\|mobile layout'` (React) | 2 passed: cloud intent/save/resume/export and mobile/provider/authentication-failure handling. The backslash before the pipe here escapes Markdown table formatting; the shell argument uses the plain pipe. |
| `SMP_LOCAL_E2E=1`, `SMP_KEEP_ACCEPTANCE_NETWORK=1`, `npm exec playwright -- test e2e/local-kubernetes.spec.ts` (React) | Passed after fixes, 3.4 minutes on the retained network. Real wizard input, own-worker interruption, retry, CA identities, ordering, channels, contract lifecycle, Put/Get, dashboard reload and peer outage/recovery. |
| `SMP_LOCAL_E2E=1`, explicit `SMP_EXISTING_OPERATION_ID`, `npm exec playwright -- test e2e/local-reconcile.spec.ts` (React) | Passed in cleanup mode: resumed saved design, another real transaction, bounded node logs, cancel confirmation, then actual namespace/PVC/channel cleanup. SQL design/history retained and result became removed/unverified. Retention mode is also available for the final running network. |

The first retained success was operation `op_def39de401d90f3f1c4c6c9ae08704ff`, transaction `3aaf2d6a3c40a141e916e44a5cf12e133a34a145d3ac8523a5c36ae697e6b539`, both peers and all three orderers at height 9. Its later cleanup was verified. A fresh retained success was `op_576ed075acf6f462d8038b373af3aa57`, transaction `17e79d8269b8664d5eff2b2bcd15a3963db3fbca61c2cbf441452d26bb9b71dc`, height 9, namespace `smp-n-968a212068be7d07`. Later verification operations append actual transactions to that network; consult the latest operation and the final retained-state section below.

## Failures found and fixed

Actual execution exposed incompatible chart CRDs, legacy Helm CRD deletion behavior, NodePort-zero endpoint discovery, multi-organization lifecycle endorsements, chaincode package/service port mismatch, missing TLS root files on retry, service readiness preceding a listening TLS socket, and Bevel's update-time `PullAlways` behavior. Failed attempts stayed failed and were not reported as successful deployments. See [audit](local-audit.md) for details. The final image adopts a matching existing chaincode service, enforces local image policy and verifies TLS readiness.

## Limits of verification

The environment was an existing Docker Desktop/kind installation with fresh Fabric namespaces. Setup was rerun end-to-end; a newly installed Windows OS, empty image cache and newly downloaded kind branch were not independently recreated. External registry availability and first-install prerequisites remain machine-dependent. The separately retained Docker-only profile was not rerun during this qualification. Real AWS/EKS/Azure, a live Okta tenant, native server hosting, production RBAC/secret storage, distributed workers, HA, backup restoration and certificate rotation were not qualified. Cloud execution remains disabled; the local development profile must not be advertised as production-ready.

Exact prerequisites, environment variables, setup/restart/test/cleanup commands and future-cloud access prerequisites are in [local Kubernetes setup](local-kubernetes.md).

## Final retained state

- Console: `http://127.0.0.1:5173`; API: `http://127.0.0.1:8080`.
- Saved network name: `Local acceptance 1792206274622`.
- Namespace: `smp-n-968a212068be7d07` in `kind-smp-qualification`.
- Latest verified operation: `op_9ac07bd1f386c3a692efb5a02082bca8`.
- Latest transaction: `aa26fd2e4deee16724616ba420883d262151f3fec8884cd490f12cd6d2426117`.
- Both peer ledgers match at height **11**; three orderers are active consenters; two CAs passed TLS checks. Eight Fabric workloads are running. Worker Jobs are completed, not application containers that need to stay running.
- `simulated=false`, `verified=true`; current workload health returned ready after the final API restart. The real snapshot and operation survived that restart in PostgreSQL.
- The final repeat browser test using the rebuilt worker passed in **16.5 seconds**, without manual Kubernetes mutation. Its existing-chaincode adoption and local image policy correction were exercised. Earlier cleanup and fresh-network tests passed separately.

In a fresh browser tab, open Networks, select the saved network above, then open its dashboard. Completed historical operations remain visible; newer operations supersede their readiness so old proofs cannot be displayed as the current deployment.
