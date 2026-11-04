# Fabric SMP — Sathonate control plane

Sathonate's Go API for designing Hyperledger Fabric networks. This is an independent Git repository. The React console lives in `E:/Sathonate/fabric-console` and communicates only through HTTP.

## What works

- **Cloud launch readiness:** the console and `scripts/check-cloud-readiness.ps1 -Probe` report missing AWS/EKS, HTTPS and Okta setup and perform explicit read-only account/cluster binding checks once configured. No cloud command runs with missing bindings, and no paid resources are created. See [account-owner requirements and setup](docs/cloud-launch.md).

- **Real local Kubernetes deployment:** the complete six-step console drives PostgreSQL-backed jobs and a Bevel Fabric operator worker on kind. CA enrollment, two peers, three Raft orderers, channel membership and a TLS Go chaincode transaction are verified. Ansible/Helm bootstrap and exact Windows instructions: [local Kubernetes qualification](docs/local-kubernetes.md). AWS/Azure provisioning remains disabled.

- Persist the UI's cloud and cluster settings in the network JSON and prepare revision-bound deployment packages with canonical network digests. Registered targets generate Bevel inputs and suspended Kubernetes resources; missing targets return blockers. See [deployment handoff](docs/deployment-handoff.md).

- Run a real local Docker network with Fabric CA identities, three Raft orderers, two peers, channel creation and a Go contract transaction check. See [local runtime](docs/local-runtime.md).
- Persist operation IDs, ordered deployment stages, audit events, idempotency keys and linked retries; query live peer/orderer/CA/chaincode inventory. See [product-spec alignment](docs/spec-alignment.md).

- Save, list, retrieve, update and archive network drafts.
- Validate configuration and topology, including references and production Raft placement rules.
- Generate deterministic review plans; clear a previous review when the design changes.
- Explicit ordering profiles: Raft for Bevel/Fabric 2.5 and native 2.5/3.x; native SmartBFT planning for Fabric 3.x/V3_0. Solo is retired and rejected.
- Persist desired configuration, revision history and review metadata across API restarts.
- Reject stale updates, duplicate active network IDs, unknown input fields, arbitrary browser origins and untrusted Host headers.

Optional Okta access-token authentication, tenant-scoped PostgreSQL persistence, a server-managed EKS target registry, read-only cloud preflight checks and non-executable Bevel candidate exports are implemented. Local development containers and CA enrollment work through a dedicated runtime. Deployment of arbitrary saved designs, remote server agents and production Fabric monitoring remain unavailable. Drafts and plans are never reported as live networks.

## Run

Requires Go 1.26 or newer. Dependencies are pinned in go.mod/go.sum.

```powershell
cd E:\Sathonate\fabric-control-plane
.\scripts\dev.ps1
```

The default local-development API listens at `http://127.0.0.1:8080` and refuses non-loopback binding. No external accounts are needed. Run the independent console on port 5173; its Vite proxy forwards `/api` requests here. Production mode requires Okta, PostgreSQL and explicit browser/Host allowlists; it is not a declaration of production qualification. See [integration configuration](docs/integrations.md).

```powershell
.\scripts\check.ps1
.\bin\fabric-api.exe -addr 127.0.0.1:8080 -data data/drafts.json
```

Do not start multiple API instances against the same data file. The default `data/drafts.json` is ignored by Git. The local store uses serialized in-process updates and temporary-file replacement. It is a development facility, not production durability or multi-process locking. Windows filesystem ACLs should protect the containing directory; POSIX file modes alone do not establish Windows access control.

## API

| Method | Path | Purpose |
| --- | --- | --- |
| GET | /api/v1/meta | Capability flags and development-mode information |
| GET | /api/v1/auth/config | Public sign-in configuration, without secrets |
| GET | /api/v1/cloud/targets | Registered targets for the authenticated tenant |
| POST | /api/v1/networks/{id}/cloud-preflight | Read-only prerequisite checks at the current revision |
| POST | /api/v1/networks/{id}/cloud-bundle | Export a non-executable candidate at the current revision |
| POST | /api/v1/networks/{id}/deploy | Start an explicit configured deployment with If-Match and Idempotency-Key |
| GET | /api/v1/deployments/{operationId} | Tenant-scoped persisted deployment snapshot and verification evidence |
| GET | /api/v1/deployments/{operationId}/health | Refresh real local Kubernetes workload availability |
| GET | /api/v1/deployments/{operationId}/logs/{node} | Read bounded redacted logs from a desired Fabric node |
| DELETE | /api/v1/deployments/{operationId} | Delete only the owned local network, retaining SQL history |
| GET, POST | /api/v1/networks | List active drafts or create a validated draft |
| GET, PUT, DELETE | /api/v1/networks/{id} | Read, update, or archive a draft |
| POST | /api/v1/networks/{id}/review | Generate/persist a review plan |
| POST | /api/v1/validate | Validate a desired specification |
| POST | /api/v1/plans | Generate an unsaved review plan |

Writes use `application/json`. Updates, archives and reviews require `If-Match: "<revision>"`. Missing revisions return 428; stale revisions return 409. Network IDs are immutable; display names can change. Archive affects only the saved design and never calls infrastructure.

See [the contract](docs/http-contract.md) and [verification/limitations](docs/verification.md). The schema namespace is `fabric.sathonate.io/v1alpha1`, owned by this product contract, not by an external deployment engine.

## Structure

```text
cmd/fabric-api/    Loopback HTTP service and graceful shutdown
cmd/fabricctl/     Offline validation/planning CLI
internal/domain/  Desired specification and offline admission rules
internal/drafts/  Local snapshot store, revisions and activity
internal/httpapi/ Transport, input boundaries and typed errors
internal/planner/ Deterministic, non-executable review plans
internal/engine/  Future execution interface; no runtime adapters yet
examples/         Synthetic fixtures, without customer credentials
```

The implementation is authored for Sathonate. External repositories inform lifecycle requirements and failure cases; no Bevel/Jenkins source is embedded in this backend. Cloud execution will integrate Bevel behind an adapter. Native execution will use a separately implemented Fabric library.

## Next backend milestone

Qualify the implemented authentication/database boundaries, modernize and qualify the Bevel adapter, then implement signed execution bundles, a durable operation scheduler, scoped workers, recovery and live reconciliation. Execution remains disabled until those requirements pass. Native hosting remains a later milestone.

## AWS EKS qualification worker

The existing-cluster adapter reuses the local Fabric lifecycle and adds role-bound target preflight, scoped worker RBAC, a channel broker and durable PostgreSQL recovery. Production cloud execution remains disabled. See [implementation map](docs/eks-implementation-map.md) and [exact Windows setup, local tests and EKS prerequisites](docs/eks-worker.md). No AWS resource creation is included.
