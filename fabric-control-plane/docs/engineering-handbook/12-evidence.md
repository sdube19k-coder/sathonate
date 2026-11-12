# 12 · Testing and acceptance

[Handbook](README.md) · [Coverage](coverage.md) · [Release gates](../rc1-readiness.md)

| Layer | Reproduce |
|---|---|
| Go contracts and regressions | From control plane, `go test -count=1 ./...` and `go vet ./...`. Default tests skip opt-in external fixtures. |
| Disposable PostgreSQL TLS/RLS | Set `SMP_POSTGRES_TLS_E2E=1` and run `go test -count=1 -v ./internal/persistence -run '^TestProductionPostgresTLSAndRoleOnDocker$'`. Requires existing local image and disposable Docker. |
| Console and customer browser | From console, `npm test`, `npm run build`, and `npx --no-install playwright test --config playwright.customer.config.ts`. Browser pilot fixtures intercept API calls. |
| Helm and RC1 | From control plane, `python -B tools/sprint8/check_helm.py --helm helm`, `python -B -m unittest discover -s tools/rc1 -v`, then use the live receipt criteria in RC1. |

| Sprint | Evidence boundary |
|---|---|
| 4 | Dynamic topologies, lifecycle and committed transactions on disposable local Fabric. See [dynamic acceptance](../sprint4-dynamic-acceptance.md) and [lifecycle acceptance](../sprint4-lifecycle-acceptance.md). |
| 5 | Peer/Raft/CA recovery, guarded retirement and cold recovery evidence is scoped to the reported fixture and artifacts. See [Sprint 5](../sprint5-acceptance.md). |
| 6 · PARTIAL | Four-node harness exists. Capacity blocked live multi-node execution. See [Sprint 6](../sprint6-acceptance.md). |
| 7 · PARTIAL | Controlled security, scheduler and builder evidence. Real Okta, historical remediation and replacement recovery remain open. See [Sprint 7](../sprint7-acceptance.md). |
| 8 · PARTIAL | Helm, local certificate verification, metrics, upgrade guards and readiness soak. Customer runtime and live upgrade qualification remain open. See [Sprint 8](../sprint8-acceptance.md). |
| 9 · PARTIAL | Customer pilot, signed controlled payment and browser evidence, PostgreSQL isolation. Greenfield grants, real payment and live provisioning remain open. See [Sprint 9](../sprint9-acceptance.md). |

RC1 requires exact-candidate receipts and independent review for customer Kubernetes, real Okta, payment sandbox before paid activation, multi-node resilience and disaster recovery. Earlier local evidence keeps its original scope. The [RC1 report](../rc1-readiness.md) identifies owner roles, prerequisites and severity.
