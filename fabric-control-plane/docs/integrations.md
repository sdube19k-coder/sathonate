# Integration boundaries

Update 2026-10-17: a dedicated kind context is now configured and the real local Bevel operator workflow is verified, including Ansible/Helm bootstrap and Fabric transactions. No company EKS target is configured. See [current local qualification](local-kubernetes.md) and [updated audit](local-audit.md). The older tooling notes below describe the cloud-fork investigation, not the local worker's capabilities.

## Verified development tooling (2026-10-16)

The Linux image `sathonate/bevel-tools:dev-20261016` now builds and runs Ansible core 2.19.2, Helm 3.19.0 and kubectl 1.34.1. `scripts/check-bevel.ps1` verifies the inspected source commit and passes the Fabric `deploy-network.yaml` syntax check using a read-only checkout and an unprivileged temporary container. See [toolchain instructions](../tools/bevel/README.md). This is not the cloud execution worker; syntax success does not validate dynamically included roles or provision infrastructure. No Kubernetes context or company EKS target is configured. The pre-existing containers were removed at the user's request; volumes and images were retained.

## Local development

Run scripts/dev.ps1 and the separate React console. No AWS account, cluster, database or Okta application is required. Authentication is explicitly local-development, one local tenant uses the ignored draft file, and the target catalog is empty. Network design, validation, ordering configuration, review and export work offline from infrastructure. Cloud checks return a missing-target blocker rather than simulated success.

## Optional deployment configuration

The API accepts `-mode production`, `-addr`, and `-cloud-config <trusted-registry.json>`. Configure these through the deployment environment rather than committing values:

| Variable | Purpose |
| --- | --- |
| DATABASE_URL | PostgreSQL connection with verified TLS in production |
| OKTA_ISSUER | HTTPS custom authorization-server issuer |
| OKTA_CLIENT_ID | SPA client ID; validated against access-token cid |
| OKTA_AUDIENCE | Distinct API audience |
| OKTA_TENANT_CLAIM | Trusted tenant claim; defaults to sathonate_tenant |
| API_ALLOWED_HOSTS | Comma-separated exact external API Host values |
| CONSOLE_ALLOWED_ORIGINS | Comma-separated HTTPS console origins |

Terminate HTTPS at a trusted reverse proxy; the Go listener itself serves HTTP and must be reachable only through the protected ingress. Do not expose its listener directly. Serve the SPA and proxy /api on the same origin. Configure the Okta SPA redirect URI at `/login/callback` and the sign-out redirect at the console origin. Tokens use PKCE; access-token storage is in memory. ID tokens are not accepted as API credentials.

Access tokens require fabric.read, a trusted tenant claim, and a mapped group: sathonate-admin, sathonate-operator or sathonate-viewer. Writes additionally require fabric.write and admin/operator role. Tenant IDs come from verified claims, never request JSON. Group and scope assignment must be controlled by the company authorization server.

Apply the schema explicitly using `fabric-api -migrate` with DATABASE_URL and a separate migration owner. Run this administrative command in local-development mode if the migration role is privileged. The serving production role must have the required table privileges, must not be superuser or BYPASSRLS, and must use verified TLS. The schema forces tenant row-level policies and records desired revisions transactionally. Real PostgreSQL integration tests require SATHONATE_TEST_DATABASE_URL; without it they skip. This database path has not yet been qualified against a running database here.

## EKS adapter contract

The trusted registry types are defined in internal/cloud/config.go. Each target belongs to one authenticated tenant and records the exact account, region, cluster, kube context, storage class, DNS domain, GitOps URL/Secret reference, digest-pinned runner image, Bevel commit/checkout, organization Vault bindings and placement selectors. It is operator configuration, not a browser-uploaded credential document. Keep actual credentials in workload identity or a secret manager. The default empty registry is deliberate.

Preflight uses bounded, read-only AWS and kubectl commands. Secret checks request object names, not data. Candidate compilation maps a validated design to Bevel variables and suspended Kubernetes Job manifests. No Kubernetes apply is performed. The `/opt/sathonate/bin/cloud-runner` command is a proposed runner contract: its implementation and image are not supplied by this milestone. Exported jobs are not runnable release artifacts and must remain suspended.

Known blockers in the inspected Bevel fork include Flux v1 resources, old accepted Fabric versions, fixed MSP naming, legacy runtime/storage assumptions, and placement selectors that are not yet enforced by its generated workloads. Supplying selectors or a certification reference does not prove enforcement or certification. No custom MSP is silently renamed. Candidate exports contain secret references and infrastructure metadata and should be handled as internal artifacts.

## Release gates

Execution is hard-disabled in API plans, target summaries and preflight reports. It needs a maintained adapter and runner, enforced workload placement, supported Fabric/Flux/EKS compatibility, image provenance, scoped identities, tenant isolation tests, durable scheduling/recovery, backup/restore tests and a real Fabric deployment with transaction verification. Okta, PostgreSQL and EKS have not been tested against company infrastructure. Configuration alone cannot release these gates.
