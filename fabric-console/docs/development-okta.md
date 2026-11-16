# Development Okta sign-in

Use the existing Sathonate Console SPA with Authorization Code and PKCE. The exact local origin is `http://127.0.0.1:5173`. Register `http://127.0.0.1:5173/login/callback` as the sign-in redirect and `http://127.0.0.1:5173` as the sign-out redirect. The assigned user needs the application sign-in policy and authorization-server policy for `openid profile email fabric.read`. Password, Okta Verify and any enrollment step are controlled by the IdP.

## Start the local API

Run this from `E:/Sathonate/sathonate/fabric-control-plane` in normal Windows PowerShell. The existing test container publishes PostgreSQL on loopback. The password is read into the API process environment without printing it.

```powershell
docker start smp-postgres
$pgUser = (docker exec smp-postgres printenv POSTGRES_USER).Trim()
$pgPassword = (docker exec smp-postgres cat /run/secrets/postgres-password).Trim()
$env:DATABASE_URL = 'postgres://' + [uri]::EscapeDataString($pgUser) + ':' +
  [uri]::EscapeDataString($pgPassword) + '@127.0.0.1:15432/fabric_smp_tests?sslmode=disable'
$env:OKTA_ISSUER = 'https://integrator-5135965.okta.com/oauth2/default'
$env:OKTA_CLIENT_ID = '0oa193gpwu6s8uJ7I698'
$env:OKTA_AUDIENCE = 'api://default'
go run ./cmd/fabric-api -mode local-development -local-oidc -deployment-mode disabled -addr 127.0.0.1:8081
```

Stop the old Vite process on port 5173 and start the console from `E:/Sathonate/sathonate/fabric-console` with:

```powershell
npm run dev -- --mode development-okta
```

The development Okta mode proxies the existing console origin to the Okta API at 8081. The normal Vite mode continues to use 8080.

## Complete login and verify the grant

1. Open `http://127.0.0.1:5173` and select **Sign in with Okta**. Complete the assigned Okta verification steps.
2. On first callback, the console may say there is no authorized project and show **Verified issuer** and **Verified subject**. Those values come from the API after validating the token. Use that identity for the exact development membership. Keep tokens, credentials and callback query parameters out of logs and chat.
3. Use the existing `fabric-membership` command with a protected grant JSON and the approved project anchor. The Sprint 1 development anchor **Browser Fabric 1543ce01** was present with record ID `330ffcc9e699c1e5b719c0b3ec50d719`. Verify the stored project scope against the approved tenant and project before granting. The administrative command requires an expiring viewer grant containing only `read`.
4. Click **Retry project authorization**, open the project, and verify authenticated reads and denied writes. Click the sign-out icon in the workspace header and verify the browser returns to the local sign-in page. Confirm a fresh sign-in follows the expected Okta policy.

If Okta requires an unavailable passkey, record the exact message and have the Okta administrator check which enrollment or sign-in rule matched the SPA. Live acceptance requires a successful callback, approved project access and a verified logout.

## Development verification · 12 November 2026

The user confirmed that Okta sign-in returned to Sathonate, the approved read-only development project opened, and header sign-out returned to the sign-in page. The verified identity was bound to an expiring viewer grant in PostgreSQL for `smp-sprint1-1543ce01 / project-a`. This verification covers the development console and its assigned MFA policy. Production Okta acceptance remains open.
