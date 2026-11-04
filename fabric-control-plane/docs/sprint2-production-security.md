# Sprint 2 production security qualification

Overall status: **BLOCKED**. Production and AWS execution remain disabled. This qualifies a controlled local security profile, not production release. Final results and source hashes: `../.logs/sprint2-evidence.json`. Failed attempts remain separate.

## Authentication and authorization

The HTTP boundary validates RS256 signatures through HTTPS OIDC discovery/JWKS, exact issuer, distinct API audience, authorized SPA `cid`, subject, tenant, project, expiry, issuance time, optional `nbf`, and a maximum one-hour lifetime. Discovery/key requests are bounded. JWKS must use the configured issuer HTTPS origin. ID tokens addressed to the SPA are rejected.

The API uses the server-owned PostgreSQL membership registry. Grants bind exact verified issuer + subject + environment to a tenant/project, role, permissions and validity window. Every authenticated request resolves current membership; database revocation takes effect on the next request. The API role requires SELECT only on this registry. A separate administrator performs explicit development-only grants anchored to an existing network and verified storage scope. No real user grant has been created: interactive Okta verification was skipped at the user's passkey step. File-backed memberships are restricted to controlled test fixtures.

The explicitly selected local OIDC mode binds the project on the server. Production claim selection must match a server-owned grant. Groups, email domains and request headers never grant permissions. Operation history reads do not start execution or claim live health.

Local tokens remain cryptographically generated, hashed server-side, identity/tenant/project/permission-bound and expiring. They require explicit `-mode local-development -local-token-grants <protected-file>`; staging, production and ambiguous selection fail closed. The fixed local developer identity cannot enter the secure runner. Production requires PostgreSQL/OIDC and still only permits `-deployment-mode disabled`. The OIDC-to-Fabric fixture uses a private test-process HTTPS issuer; it adds no development signing key or production deployment path.

Sources: `internal/identity/{okta,membership,local_tokens}.go`, `cmd/fabric-api/main.go`, `internal/httpapi/{api,deployment}.go`.

## Real Okta development configuration

The supported backend variables are `OKTA_ISSUER`, `OKTA_CLIENT_ID`, `OKTA_AUDIENCE`, and optional `OKTA_TENANT_CLAIM` / `OKTA_PROJECT_CLAIM`. Use the explicitly selected `-mode local-development -local-oidc` with a loopback listener and PostgreSQL. Production execution stays disabled. `tools/sprint2/serve_okta.py` loads public configuration from the ignored private configuration file and passes the database credential through the child environment only.

The console is a browser SPA at `http://127.0.0.1:5173`, with callback `/login/callback` and logout return to the origin. It initially requests `openid profile email fabric.read` using Authorization Code with PKCE. No write scope is requested. Real discovery/JWKS and the authorization page were reached. The second supplied System Log export identified ten matching SPA failures using the approved email twice, an unknown actor and `VERIFICATION_ERROR`. The agent had appended to a prefilled username; this was corrected by replacing the entire field and visually checking it. A fresh PKCE flow now recognizes the correct username and offers Okta Verify or Password. No password reset or authentication-policy weakening is justified by that finding. After user verification, the corrected flow required passkey enrollment (**Required now**). The user explicitly deferred further Okta work. No enrollment/authentication policy was changed and no real-user grant was created. Callback, actual access-token acceptance, authenticated reads and logout remain **BLOCKED**. Do not treat controlled OIDC fixtures as live Okta acceptance.

The uniquely verified existing development tenant is `smp-sprint1-40ec3c47`, project `project-a`, network `070e00ff6b0bbde249dbc1a73bf16632`. A future read-only development grant requires a cryptographically verified issuer + subject, not the supplied email. Its completed deployment history is recorded history; the original disposable cluster has been removed.

## Issuer and immutable material

Only an authorized persisted secure profile reaches the issuer. Fixed node/admin/client identities use protected random registrar credentials. Registration credentials persist before ambiguous registration retry. Unsupported issuance/rotation routes are absent; legacy fallback is refused. Identity Secrets must be immutable and match issuer/namespace ownership. Existing conflicting/foreign material cannot be adopted.

Before reference publication, validation checks ECDSA key pairing, validity/chain, deployment CA public-status root, exact CN, required Fabric role OU, signing usage, TLS SAN/server EKU and denied leaf/root fingerprints. Self-consistent bundles from an unrelated CA fail. Registrar material is validated before initial persistence.

Trusted runtime `deniedCertificateSHA256` and `deniedPublicKeySHA256` contains lowercase SHA256 DER public leaf/root certificate fingerprints and SHA256 SubjectPublicKeyInfo fingerprints, never keys or customer input. Public-key fingerprints also reject reuse of an exposed key under a reissued certificate. Unknown historical exposure inventory is an unresolved incident gate. Fresh namespaces issue new CAs and cannot adopt retained namespaces. Public-certificate inventory is mandatory before migration or reuse.

The unchanged candidate mounts Secrets. Public CRs, Helm values/history and ConfigMaps carry references/public material. CA client configuration/private MSPs stay in memory volumes with mode 0600. Credentials never enter CA client/protected HLF argv. Inspection compares actual private values in memory and exports only booleans/counts/hashes. No historical retained raw keys/logs are read.

Atomicity is one Deployment template selecting a validated immutable bundle. Secret creation, CR updates, Helm storage, rollout and acknowledgement are distinct writes. Checkpoints/idempotent forward recovery handle ambiguity. Helm rollback cannot reverse Fabric channel/MSP/consenter state; Recreate can interrupt service. Six peer-TLS failure cases do not qualify signing, CA/root, registrar, operations/admin or Raft rotation. Retention, restore and garbage collection remain release gates.

Sources: `internal/kubernetesfabric/{secure_issuer,authorized_material,runtime}.go`, `tools/controller-atomic/{atomic.go.tmpl,qualify.py}`, `controller-atomic-qualification.md`.

## Kubernetes boundaries

| Identity | Granted authority | Excluded authority |
| --- | --- | --- |
| trusted runner | guarded disposable namespace/RBAC/policy/quota/workload creation; exact-owned Job cancellation | remote/retained mutation; production execution |
| secure-controller | four Fabric CRs/status/finalizers; Helm Secret/ConfigMap/service/PVC/deployment reconciliation; pod observation/deletion, events, leases | foreign namespaces, nodes, RBAC creation, cluster channels |
| secure-issuer | named Secret get/create/patch/update; observe pods/services/deployments; four Fabric CR get/list/watch/create/patch/update | Secret list/delete, foreign resources, pod exec, RBAC, cluster resources |
| default Fabric account | no API grants; token automount disabled | no Fabric pod API credential |

Namespace-wide controller Secret access remains necessary for pinned Helm/cache behavior. One exclusive network/tenant/project per namespace is not per-identity isolation. A compromised controller can access its namespace credentials. Cluster/node compromise, encryption and administrator trust need separate production controls.

Containment precedes workloads: default-deny ingress/egress; same-namespace Fabric TCP 7050–7054, 9443, 9999; CoreDNS UDP/TCP 53; API TCP 443/6443 only for labeled controller/issuer pods to discovered control-plane/service IPs. No global egress CIDR. Server-owned approved digest-pinned runtime images are part of durable binding; customers cannot select images or mutate bindings. Admission-enforced registry allowlists remain a target production gate.

Quota: 40 pods, 20 PVCs, 32Gi storage, 200 Secrets, 100 ConfigMaps, 32 Services; requested CPU/memory 16/16Gi, limits 40/40Gi. LimitRange supplies defaults and caps each container at 2 CPU/2Gi. Kubernetes v1.34 **baseline** Pod Security is enforced. Controller/issuer are non-root, no escalation, dropped capabilities, RuntimeDefault seccomp; issuer root filesystem is read-only. Restricted admission compatibility for every Fabric container is not qualified.

Default kind CNI is not isolation evidence. `tools/sprint2/isolation.py` prepares separate Calico v3.31.4 from a SHA256-verified official manifest, resolving immutable image digests. It never installs CNI into a retained cluster. Probes require own-network positive connectivity, foreign ingress denial, own-to-foreign egress denial with a live foreign listener, actual Forbidden admission responses and absence of Fabric API tokens. Lifecycle exercises allowed DNS/API/Fabric paths. Fixture readiness and policy enforcement are separate results.

References: [kind CNI configuration](https://kind.sigs.k8s.io/docs/user/configuration/), [Calico kind installation](https://docs.tigera.io/calico/latest/getting-started/kubernetes/kind).

## Production checklist and unresolved gates

- Explicit environment, verified DB TLS on every fallback, separate migration owner/executor. API roles cannot be superuser, BYPASSRLS, CREATEROLE, CREATEDB, replication, privileged-role members (including predefined `pg_*` capabilities) or application-table owners. FORCE RLS and transaction-local scopes protect repositories/journals. Trusted recovery execution is a separate capability. PostgreSQL documents that server-file/program roles can bypass database checks; these memberships are denied too: [predefined roles](https://www.postgresql.org/docs/18/predefined-roles.html). Direct function grants and service deployment must still pass the production DBA privilege review.
- Existing revisions, operation uniqueness, idempotency keys, bounded worker/HTTP timeouts, restart adoption and cancellation remain. Retry revalidates target/namespace binding. Readiness/shutdown remain bounded; cached observations are not live transaction proof.
- Fixed-category secure errors, no diagnostic payloads or development grants in production configuration. Production execution remains disabled even with valid OIDC.
- Approve/pin images and build inputs; verify binary/runtime manifests, signed provenance and dependency scanning before release. Local archive normalization is not registry-publishing qualification.
- Qualify target-cluster CNI/admission, encryption at rest, external secret delivery, backup restoration, restricted pod compatibility, actual DB verified-TLS/restricted-role service deployment, protected audit retention and incident disposition.

## Reproduction

Windows PowerShell, repository root, Docker Desktop Linux engine and the existing development PostgreSQL fixture:

```powershell
$env:GOCACHE='E:/Sathonate/.cache/go-build'
$env:GOPATH='E:/Sathonate/.cache/go'; $env:GOPROXY='off'
$env:TMP='E:/Sathonate/.cache/resume-tmp'; $env:TEMP=$env:TMP
$env:GOOS='linux'; $env:GOARCH='amd64'; $env:CGO_ENABLED='0'
go build -trimpath -o bin/fabric-worker-secure ./cmd/fabric-worker
Remove-Item Env:GOOS,Env:GOARCH,Env:CGO_ENABLED
go build -trimpath -o bin/fabric-api-sprint2.exe ./cmd/fabric-api
docker build -f tools/kubernetes/Dockerfile.secure -t sathonate/fabric-worker:sprint2 .
$env:SMP_QUALIFICATION_PREFIX='sprint2'
$env:SMP_QUALIFICATION_WORKER='sathonate/fabric-worker:sprint2'
$env:SMP_QUALIFICATION_CALICO='1'; $env:SMP_SECURE_OIDC_FIXTURE='1'
python tools/sprint1/qualify.py
go test ./... -count=1
go vet ./...
python tools/sprint2/postgres.py
python tools/sprint2/startup.py
$env:SMP_POSTGRES_TLS_E2E='1'
go test ./internal/persistence -run ^TestProductionPostgresTLSAndRoleOnDocker$ -count=1 -v
Remove-Item Env:SMP_POSTGRES_TLS_E2E
python -m unittest discover -s tools/controller-atomic -v
python -m unittest discover -s tools/controller-lab -v
python -m unittest discover -s tools/engineering -v
python -m unittest discover -s tools/sprint2 -v
```

The harness reads the existing development DB password only in memory, never argv/output. Run shared DB suites serially after API recovery. Keep runtime suites sequential on this host. Original local/scoped tests hardcode the retained cluster/original controller: do not execute those against that target under the read-only restriction. `python tools/sprint2/legacy_regressions.py` creates a separate cluster and uses a temporary Go overlay changing only the fixed cluster-name literals. It preserves loopback/context/provider checks, records source/adapted hashes, runs a separate unchanged legacy controller image, withholds legacy diagnostic bodies and removes only its fixture. These are fixture-adapted regressions, not production controller qualification. Source files and retained resources are unchanged.

Atomic reproduction: `python tools/controller-atomic/qualify.py --kind E:/Sathonate/tools/kind.exe --cache E:/Sathonate/.cache --source E:/Sathonate/dependencies/bevel-operator-fabric --image sathonate/controller-candidate:atomic --output .logs/sprint2-atomic-kind.json`. Only its new guarded cluster is removed. During that run, `python tools/sprint2/observe_ack.py --cluster <new-atomic-cluster>` independently observes the final held acknowledgement, public certificate, controller availability, Helm revision, both peer pod UIDs and container restart counters. The main harness separately commits/queries transactions on both peers. Retained health is read-only Deployment readiness, not fresh transaction evidence.

See `sprint2-incident-migration.md`. Sprint 3 must not start until this security gate closes and is explicitly authorized. Sprint 1 backend baseline is `aed4145`; console completion `e230b29` supersedes the older prompt's blocked statement. This sprint does not recertify the console.
