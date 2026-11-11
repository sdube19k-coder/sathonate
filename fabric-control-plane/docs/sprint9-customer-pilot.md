# Sprint 9 customer pilot API and onboarding

The customer pilot is disabled by default. The API accepts `-customer-pilot` only with production Okta authentication, PostgreSQL, and `-deployment-mode disabled`. This flag exposes account management. Payment routes and customer Kubernetes execution remain unavailable.

## Onboard a pilot customer

1. Confirm the tenant, the exact verified Okta issuer and subject, and an existing server-owned project membership for the initial owner. The owner grant must be active with administrator role and write permission. Configure the existing MFA and PKCE requirements. Keep initial browser sign-in read-only. An owner may explicitly request `fabric.write` using **Authorize account changes in Okta**.
2. Apply the application schema with the existing administrative migration procedure. Use the following grants with approved existing role names. Run these commands as the database owner/DBA. Keep the customer administrator login separate from the API login and the OIDC identity-grant writer.

   ```sql
   CREATE ROLE sathonate_customer_admin NOLOGIN;
   GRANT sathonate_customer_admin TO pilot_admin_login;
   GRANT SELECT, INSERT, UPDATE (payload) ON public.sathonate_customer_accounts TO sathonate_customer_admin;
   GRANT SELECT, INSERT ON public.sathonate_customer_changes TO sathonate_customer_admin;
   GRANT SELECT ON public.sathonate_oidc_memberships TO sathonate_customer_admin;
   GRANT SELECT, UPDATE (payload) ON public.sathonate_customer_accounts TO fabric_api_login;
   GRANT SELECT, INSERT ON public.sathonate_customer_changes TO fabric_api_login;
   ```

   Provision the administrator connection through a protected environment with `CUSTOMER_ADMIN_DATABASE_URL` and `sslmode=verify-full`. Both logins require existing schema access. The API also needs its existing grants. Only an optional metering operator needs read access to the scoped operation and deployment history.
3. Bootstrap the organization using the separate role. Use real approved IDs in place of the placeholders.

   ```powershell
   go run ./cmd/fabric-customer-admin -action bootstrap -tenant TENANT -project EXISTING_PROJECT -issuer ISSUER -subject VERIFIED_SUB -name 'Customer organization'
   ```

4. Start the API with `-mode production -deployment-mode disabled -customer-pilot`. Sign in, open the authorized workspace and select **Customer account**. The account owner can start one 14-day trial, request projects, and invite an exact subject from the same issuer. Invitation acceptance requires the recipient's own existing project grant in that tenant. Invitations expire after 48 hours. Provide the invitee with the pilot console URL through the approved channel. The account shows pending invitations after sign-in.
5. Project requests create a durable ID in `awaiting-identity-approval`. Use the existing identity administrator to provision and verify the project's independent grant. With the exact owner and new project ID, use the separate customer administrator role to activate the catalog record. The command verifies the active owner grant for the exact project.

   ```powershell
   go run ./cmd/fabric-customer-admin -action approve-project -tenant TENANT -project NEW_PROJECT -issuer ISSUER -subject VERIFIED_SUB -approval REVIEW_REFERENCE
   ```

   The current identity-grant model still requires a pre-existing network anchor. Greenfield grant creation and switching across multiple server-bound project grants require a separate approved identity workflow. Project requests and activation never create Kubernetes workloads.

## Versioned HTTP API

Requests use bearer authentication. The server derives tenant, issuer, subject and active project from verified authentication and the server-owned membership. Mutations use `Content-Type: application/json` and exactly one `Idempotency-Key` of 16–128 allowed identifier characters. Replaying the same key and command returns the original successful effect. A different command with that key returns 409. Reuse the key for an ambiguous response.

| Route | Permission | Response |
| --- | --- | --- |
| `GET /api/v1/customer` | Authenticated member or exact pending invitee | 200 with scoped `account`, `plans`, `canManage`, `paymentsEnabled: false`, `infrastructureEnabled: false`. Only the billing owner can see invoices, usage and all catalog projects. |
| `POST /api/v1/customer/actions` | Billing owner with administrator role, granted write permission and write scope | 204. Body uses `action` plus optional `name` or `reference` as below. |
| `POST /api/v1/customer/invitations/accept` | Exact authenticated recipient with a read grant in the tenant | 204. Accepts only `{"action":"accept-invitation","reference":"INVITE_ID"}`. |
| `GET /api/v1/meta` | Existing workspace access | `customerPilot` advertises the account feature. |

| Action | Additional field |
| --- | --- |
| `rename-organization` | `name` |
| `start-trial` | None |
| `request-project` | `name` |
| `invite` | `reference` is the verified recipient subject for the configured issuer |
| `revoke-invitation` | `reference` is the invitation ID |

401 indicates missing or invalid authentication. 403 indicates denied authorization or entitlement, 404 indicates a disabled pilot, 400 indicates invalid input, 409 indicates a conflicting transition, and 503 indicates a backend failure. The API rejects unknown body fields, including supplied tenant, provider or card data. Customer-enabled deployment submissions require active project entitlement as well as existing deployment checks. Recovery and cleanup use their existing safeguards independently of billing state.

## Payment and metering boundaries

The server catalog has a zero-cost 14-day pilot with three project slots and a Team fixture catalog entry at 4,900 USD cents monthly with 25 slots. Prices and limits are fixtures pending approved commercial terms. Usage measures verified Sathonate operations. The product never rates or invoices customer-owned Kubernetes instances, networking, storage or other infrastructure.

Provider adapters receive opaque references only. The signed webhook fixture uses an HMAC over the delivery timestamp and exact body. Redelivery uses a fresh signature and the original event identity. The service atomically binds events to the registered customer and tenant, writes subscription/invoice transitions, and records the event digest for retry and audit. The standalone fixture contract is separate from any provider's live signature specification. It has no production ingress. A hosted-session adapter validates the exact approved HTTPS origin and gives the provider a stable idempotency key. It has no customer-facing route.

The administrative metering path reads the stored job and deployment result for an exact project scope. It requires a non-simulated, successful and verified customer Kubernetes receipt with a committed transaction ID. Current local, mock and EKS qualification receipts are rejected. Metering currently records usage without generating invoices. Provider account mapping, activation, reconciliation, refunds, taxes and customer receipts are release dependencies.

## Pilot acceptance and release checklist

- Check owner sign-in with MFA, read-only account access, explicit write-scope request, unauthorized project access denial and logout. Confirm account access fails for an authenticated foreign tenant.
- Bootstrap one organization, replay trial activation, request a project twice with the same key, exhaust the trial slot limit and verify expiry denies new deployments. Confirm project approval binds to the exact independent membership.
- Invite a verified subject, deny a different issuer, subject and tenant, verify expiry/revocation, then accept with a viewer grant. Verify that invoices, usage and unrelated projects remain private.
- Use only provider test accounts after approval. Exercise payment activation, failed payment, invoice update, signed replay after redelivery, wrong provider customer, out-of-order delivery, reconciliation, cancellation, hosted portal recovery and owner-only access. Confirm account and platform logs exclude raw payloads, signatures, hosted session URLs and credentials.
- Before infrastructure activation, qualify an approved customer-owned Kubernetes target and its identity, billing separation, transaction verification, metering backfill and recovery. Keep payment and infrastructure release decisions separate.
- Confirm pilot privacy and retention, production support procedures, audit export, alerting and rollback before opening to customers. Keep prior Sprint 6–8 live qualification statuses.
