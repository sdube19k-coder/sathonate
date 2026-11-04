# Running local Fabric

The console's **Run Fabric locally** page controls a real, dedicated development network. It is separate from saved cloud/private-server designs. This profile uses official Fabric 2.5.15 binaries and Fabric CA 1.5.15 images, without cryptogen or Bevel. The Go orchestrator and smoke contract are Sathonate code.

The fixed topology is three TLS Fabric CA servers (orderers/org1/org2), three Raft orderers, one peer per organization, a CLI container and a TLS Go chaincode service: ten containers. All published ports bind 127.0.0.1. Local Raft members share one machine and offer no machine-level availability.

Start Docker Desktop's Linux engine. Run scripts/check.ps1 to build the API and the statically linked Linux/amd64 contract; scripts/dev.ps1 also builds the contract before starting the API. The console runs from the separate React repository. Go/npm dependencies and official Docker images require network access on the first build/pull. Current profile targets Linux/amd64 Docker.

Click **Start local network**. The backend registers/enrolls CA administrators, node MSPs and TLS identities; generates configtx with three consenters; joins orderers through osnadmin; joins both peers; installs/approves/commits the smoke contract; and submits an endorsed write, waits for both peers' commit events, then queries its exact value from both peers. Readiness requires successful verification and all ten project containers running. The page displays a real transaction ID and node logs. **Run transaction check** repeats the application check.

**Stop network** stops only the sathonate-local Compose project. **Start local network** reuses CA databases, identities and channel ledgers. It reconciles partial setup without deleting resources. A stopped/interrupted operation is never presented as ready. API process restart does not stop Docker containers. Do not run multiple API processes against this runtime directory. Durable distributed scheduling is a separate production requirement.

Runtime artifacts, generated CA credentials and keys are under ignored data/local-fabric, separate from source. Ledger data uses named Docker volumes. Protect the E: runtime directory with Windows ACLs; POSIX modes do not establish Windows access controls. These identities are for local development. The registrar bootstrap command is visible to local Docker administrators; the HTTP API and event stream never return bootstrap credentials. Node containers do not receive the Docker socket; the tools container has privileged access to development identities for lifecycle commands. This is not a production identity-management model.

## API

Lifecycle responses now return HTTP 202 with a durable operation object (operationId/status/kind/stages), recorded before execution. Supply Idempotency-Key to deduplicate submissions. GET /api/v1/operations lists local tenant history; GET /api/v1/operations/{id} returns one record; /events?after=N returns ordered stage events after a sequence; POST /api/v1/operations/{id}/retry creates a linked reconciliation attempt for failed/interrupted jobs. Runtime routes and local operation routes are unavailable in production mode.

GET /api/v1/local-runtime/health queries actual channel heights/hashes, active Raft consenters, TLS CA availability and the committed chaincode definition. It does not submit a transaction or infer service health solely from container process state. The console shows last-check time and actionable alerts.

| Method | Route | Effect |
| --- | --- | --- |
| GET | /api/v1/local-runtime | Actual container observations, last verification and setup events |
| POST | /api/v1/local-runtime/start | Asynchronously create/reconcile the dedicated profile |
| POST | /api/v1/local-runtime/stop | Stop project containers; retain identities and ledgers |
| POST | /api/v1/local-runtime/test | Submit, wait for commit and query the smoke write |
| GET | /api/v1/local-runtime/logs/{node} | Bounded logs for peer1/peer2/orderer1/orderer2/orderer3 |

All mutations require application/json. Runtime routes are unavailable in production/Okta mode. No user-supplied shell scripts, image names, host paths or executable templates are accepted. Concurrent lifecycle operations return 409. Initial pulls/enrollment can take several minutes; the UI polls observed status. State and events survive API restarts; interrupted operations require an explicit retry.

## Verification

Locally verified with real Docker containers: CA enrollment, three orderer participation joins, both peer joins, contract lifecycle approval/commit, TLS chaincode service, endorsed writes and query on both peers. Go checks cover topology/port/socket boundaries, deterministic package identity and interrupted operation handling. The browser integration test runs an actual transaction when the local network is ready; otherwise it explicitly skips.

This milestone implements the fixed development profile. Custom topology deployment, native multi-host orchestration, cloud deployment, CA rotation/revocation, disaster recovery and production qualification remain separate work.
