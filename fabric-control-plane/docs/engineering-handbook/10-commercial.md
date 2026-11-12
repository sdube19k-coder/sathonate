# 10 · Commercial operations

[Handbook](README.md) · [Onboarding runbook and API](../sprint9-customer-pilot.md) · [Billing sequence](03-workflows.md)

**Why.** Control access to Sathonate platform operations separately from customer infrastructure charges and from Fabric identity authorization.

1. A separate administrator bootstraps an organization against an existing verified owner/project grant. The account owner can request a project, invite an exact subject and start one 14-day trial. A pending project needs an independent identity approval before activation.
2. Deploy admission and ordinary retry check the exact active project and subscription entitlement. Recovery and cleanup keep independent authorization. A pilot cannot activate an infrastructure runner.
3. The provider boundary accepts opaque account identifiers. The controlled webhook verifies HMAC over timestamp and exact body, validates customer binding, rejects stale/conflicting transitions and appends a change atomically. Hosted URLs must match the approved HTTPS origin. These adapters have no production ingress or checkout.
4. RC1 metering reads scoped, successful, verified customer Kubernetes journal receipts with transaction IDs. It rejects mock/qualification/foreign receipts, commits one usage event per operation and can replay the bounded window. It does not charge for customer cloud instances, storage or traffic. No live executor supplies qualifying production receipts yet.

The PostgreSQL aggregate stores subscription, invoice summaries and usage periods; idempotency and audit live in the changes table. For an ambiguous client write, reuse the original key. For metering interruption, rerun the same bound sweep. Provider reconciliation, settlement and refunds remain release work.

[Actions](../../internal/customer/actions.go) · [Provider](../../internal/customer/payment.go) · [Hosted tests](../../internal/customer/hosted_test.go) · [Meter](../../internal/customer/meter.go) · [Replay tests](../../internal/customer/meter_test.go) · [RC1](../rc1-readiness.md).
