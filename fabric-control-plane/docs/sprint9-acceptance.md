# Sprint 9 acceptance

**Overall PARTIAL.** Work started from verified GitHub main `e10dabd`. Sprint 6, 7 and 8 remain PARTIAL. Their blocked live gates were not rerun.

| Gate | Status | Evidence and boundary |
| --- | --- | --- |
| Account onboarding and invitations | **PASS in controlled acceptance** | Server-owned account bootstrap, exact issuer/subject binding, owner-only administration, 48-hour invitation expiry, cross-tenant and cross-issuer denial, and viewer invitation acceptance were exercised. The pilot requires an existing approved project grant. Greenfield identity onboarding and live Okta remain open. |
| Project provisioning and entitlements | **PARTIAL** | Durable request, slot enforcement, exact project approval, trial expiry, and denied deployment without an active project passed. The approved catalog record and existing Fabric identity grant remain separate. No customer Kubernetes resources were provisioned. |
| Plans and billing lifecycle | **PASS in controlled acceptance** | One 14-day pilot, entitlement expiry, paid-plan activation, past-due handling, immutable invoice amounts, open/paid transitions, stale event rejection, and matching event redelivery passed. Provider settlement and commercial terms remain unapproved. |
| Payment adapters and PCI boundary | **PARTIAL** | The isolated HMAC fixture rejected missing, stale and foreign signatures or account bindings, rejected conflicting replay, and accepted matching fresh redelivery. Hosted adapter tests required an exact approved HTTPS origin and reused the provider idempotency key. Production API exposes neither payment ingress nor checkout. No card fields or raw webhook payloads are persisted. |
| Usage metering | **PARTIAL** | Metering tests accepted one verified scoped receipt and rejected simulated, failed, foreign, qualification-mode and transaction-less results. Replay produced one usage entry. Administrative ingestion is present. Automatic production metering and backfill await a qualified customer execution source. Customer cloud costs are excluded. |
| Tenant isolation and audit durability | **PASS** | Disposable PostgreSQL 17.6 over verified TLS passed forced RLS checks, eight concurrent same-key calls, one committed trial, project persistence, and denial of runtime audit edits or bootstrap. Signed OIDC fixture tests rejected unauthenticated, foreign-tenant and unentitled API actions. |
| Customer console | **PASS in browser fixtures** | Three Edge Playwright scenarios passed. They covered owner retry with the same idempotency key, pending project rendering and invoice visibility, invitee acceptance with owner controls hidden, and navigation hidden while disabled. The real Okta write-scope redirect remains untested. |
| Release activation | **BLOCKED** | Requires approved payment provider configuration and reconciliation, verified production Okta and greenfield project grant workflow, and approved customer Kubernetes/usage acceptance. Payments and live infrastructure execution remain disabled. |

## Reproduce and inspect

From the control-plane directory:

```powershell
go test -count=1 ./...
go vet ./...
$env:SMP_POSTGRES_TLS_E2E = '1'
go test -count=1 -v ./internal/persistence -run '^TestProductionPostgresTLSAndRoleOnDocker$'
```

The PostgreSQL test uses an existing local image, a new loopback-bound container and temporary credentials. It removes the test container and its anonymous volumes. No kind cluster, retained Kubernetes resources or paid services were used. No disposable fixture container remained after the run.

From the console directory:

```powershell
npm test
npm run build
npx --no-install playwright test --config playwright.customer.config.ts
```

Go and vet passed. The console passed 31 unit tests, built successfully and passed all three browser fixtures. The build warned about a minified JS chunk above 500 kB. Browser calls used intercepted responses and did not contact a live identity or payment provider.

[Onboarding, API contract and release checklist](sprint9-customer-pilot.md) · [Customer domain and provider tests](../internal/customer/customer_test.go) · [Signed OIDC API tests](../internal/httpapi/customer_test.go) · [PostgreSQL acceptance](../internal/persistence/customer_postgres_test.go) · [Browser scenarios](../../fabric-console/e2e/customer-pilot.spec.ts)
