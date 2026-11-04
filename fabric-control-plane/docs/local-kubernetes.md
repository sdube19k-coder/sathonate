# Fabric SMP: real local Kubernetes qualification

Company: **Sathonate**. The React console and Go control plane are separate Git repositories. This profile creates real Fabric resources. AWS and Azure remain design-only and no paid resources are created.

## Qualified profile and boundaries

Windows 11, PowerShell 7, Docker Desktop with Linux containers/WSL2, kind 0.33.0 with Kubernetes 1.34.0, PostgreSQL 17.6, Bevel Fabric operator/CLI 1.14.0 at `cd236ccc61d1d159b3dadb4625d08bb5682a8451`, Fabric 2.5.15 and CA 1.5.15. Ansible 12/core 2.19.2 and Helm 3.19.0 bootstrap the operator; the operator reconciles its Fabric Helm charts. The worker uses the Bevel CLI and official Fabric peer binary for channel and chaincode lifecycle.

The older reference Bevel fork uses obsolete Flux and incompatible schemas. This implementation does not copy its deployment scripts or silently substitute Fabric version strings. It uses the pinned upstream operator workflow that supports CA enrollment, Raft, channel participation, peer joining and external chaincode.

The small profile has two organizations, two CAs, one peer per organization, three Raft consenters in one ordering organization, one channel and a TLS Go chaincode service `smp_smoke`. No cryptogen, Istio, cloud load balancer or public ingress is involved. Solo and BFT cannot execute in this local profile. Production and arbitrary topology execution remain unsupported.

Allocate **at least 6 GB Docker memory** (the tested engine reports approximately 5.8 GiB), preferably 8–12 GiB, 4+ CPUs and 30 GiB free disk. A 16 GiB+ Windows host is recommended. Registry downloads need Internet access. Run one qualification network at a time on smaller hosts. A single kind node provides no host failure tolerance despite three Raft processes.

Local development uses a loopback API without enterprise authentication and a dedicated worker service account with cluster-admin. The runner verifies a loopback API, explicit kind context and exact Docker kind provider ID before mutations. **Do not expose this profile to clients or use its privileges in shared/production clusters.** Okta configuration and production PostgreSQL TLS/role restrictions exist separately; this run does not qualify a live Okta tenant, cloud deployment, HA, backup restoration or multi-process job scheduling.

## Windows prerequisites and first setup

Install Git, Go (the version required by `go.mod`), Node.js LTS/npm, PowerShell 7 and Docker Desktop. Enable Linux containers and start Docker. Docker Desktop includes `kubectl`; ensure it is on PATH. Use a non-C drive:

```powershell
Set-Location E:\Sathonate\fabric-control-plane
docker info
git --version
go version
node --version
npm --version
kubectl version --client
pwsh -NoProfile -File .\scripts\setup-local.ps1 -Workspace E:\Sathonate
```

`setup-local.ps1` downloads kind with release checksum verification, clones the exact Bevel tag/commit, creates only `smp-qualification`, builds the pinned tool/worker/chaincode images, loads them into kind, runs the Ansible/Helm bootstrap, starts loopback PostgreSQL and migrates the database. It preserves an existing database volume and password. It does not delete existing clusters, containers or volumes. Review any reserved-name conflict instead of replacing it.

The bootstrap keeps matching CRDs separate from Helm upgrade ownership. It rejects legacy releases that own CRD templates: removing CRD templates during an upgrade deletes CRDs and their networks. Never bypass this check on a populated cluster. Source charts also contain stale CRDs; the bootstrap uses `config/crd/bases` from the same pinned source commit.

The database password is generated into `E:\Sathonate\.cache\local-runtime\postgres-password` with a restricted Windows ACL. It is mounted into PostgreSQL as a password file. Kubeconfigs live outside the repositories. Do not print these files, commit them, include them in ordinary network JSON or paste them into chat. Local Kubernetes Secrets and kind storage are development storage, not a production secret manager.

## Run the product

Terminal 1:

```powershell
Set-Location E:\Sathonate\fabric-control-plane
pwsh -NoProfile -File .\scripts\run-local.ps1
```

Terminal 2:

```powershell
Set-Location E:\Sathonate\fabric-console
npm ci
npm run dev
```

Open `http://127.0.0.1:5173`. Choose **Local Kubernetes**, check the configured runner, continue through Kubernetes Configuration, Network Design, Review, Create Network, Deployment Progress and Network Dashboard. The local profile fixes the supported topology; its network name can be changed. Cloud inputs are saved as non-secret intent and do not enable cloud execution.

Deployment input is the revision-bound canonical `FabricNetwork` JSON persisted by the API. Only this non-secret JSON is mounted into the worker ConfigMap. Enrollment credentials and administrator wallets are Kubernetes Secrets. Progress comes from actual worker JSON events and Kubernetes Job status, not timed simulated stages. Success requires a committed `Put`, `Get` from both peers, matching peer ledger hashes/heights, three active/caught-up ordering participants, and TLS CA checks. Database-backed operation IDs survive API restart. An interrupted operation requires an explicit retry; a retry cancels its old worker before reconciling retained resources.

`Run Transaction Check` creates another reconciliation/verification operation and real transaction. Node logs are limited to desired nodes and redacted. Health refresh checks workloads; ledger heights and transaction IDs remain explicitly labelled as the **last verification**, until another transaction check completes. Failed/unavailable workloads must not be inferred healthy from cached controller status.

## Configuration

| Setting | Local value / meaning |
| --- | --- |
| `DATABASE_URL` | Read privately by `run-local.ps1`; loopback `fabric_smp` database. Never place in source. |
| `-deployment-mode local-kubernetes` | Explicitly enables real dedicated kind execution. Default is `disabled`. |
| `-local-kubeconfig` | `E:\Sathonate\.cache\smp-qualification.kubeconfig` |
| `-local-worker-image` | `sathonate/fabric-worker:local`; built and fully loaded before submitting a Job. |
| `-deployment-mode mock` | Isolated simulated workflow; results are labelled mock and never verified Fabric evidence. |
| `-enable-cloud-execution` | Defaults false. True currently fails startup because cloud execution is not qualified/implemented. |
| `SATHONATE_TEST_DATABASE_URL` | Dedicated disposable test database, not the running product database. |
| `SMP_LOCAL_E2E=1` | Opt into real Kubernetes browser acceptance. |
| `SMP_KEEP_ACCEPTANCE_NETWORK=1` | Retain successful acceptance network for inspection; otherwise the test deletes its own network. |
| `SMP_KUBECONFIG` | Override explicit acceptance-test kubeconfig path. |
| `SMP_DOCKER_E2E=1` | Opt into the separate older Docker-only profile; do not run it concurrently on a small host. |

## Tests

With API and console running:

```powershell
Set-Location E:\Sathonate\fabric-control-plane
go test ./...
go vet ./...
docker exec smp-postgres createdb -U smp fabric_smp_tests # once only
$dbPassword=[IO.File]::ReadAllText('E:\Sathonate\.cache\local-runtime\postgres-password').Trim()
$env:SATHONATE_TEST_DATABASE_URL="postgres://smp:$dbPassword@127.0.0.1:15432/fabric_smp_tests?sslmode=disable"
go test ./internal/persistence ./internal/deployment -count=1
Remove-Item Env:SATHONATE_TEST_DATABASE_URL
Remove-Variable dbPassword
Set-Location E:\Sathonate\fabric-console
npm run build
npm test
npm run format:check
npm exec playwright -- test e2e/pdf-flow.spec.ts -g 'PDF cloud|mobile layout'
$env:SMP_LOCAL_E2E='1'
$env:SMP_KEEP_ACCEPTANCE_NETWORK='1'
npm exec playwright -- test e2e/local-kubernetes.spec.ts
Remove-Item Env:SMP_LOCAL_E2E,Env:SMP_KEEP_ACCEPTANCE_NETWORK
```

Playwright uses installed Microsoft Edge. The real test creates a unique design through the UI, interrupts only its worker Job, retries through the UI, verifies actual Fabric evidence, reloads the dashboard, checks redaction, stops/restores one owned peer to check degraded health, and optionally cleans up. It does not mock the API or Fabric. Do not run PostgreSQL executor tests against `fabric_smp`: opening a journal marks unfinished jobs interrupted, and tests use their own database.

To test resuming, another transaction and the UI's destructive cleanup confirmation against an explicitly selected **test** operation, set `SMP_LOCAL_E2E=1` and `SMP_EXISTING_OPERATION_ID=<that-test-operation-id>`, then run `npm exec playwright -- test e2e/local-reconcile.spec.ts`. This test deletes that selected network after proving another transaction and log access. It never selects a network implicitly. The primary acceptance test can then create a fresh retained network.

## Restart, inspect and cleanup

```powershell
docker desktop start --timeout 30
docker start smp-postgres smp-qualification-control-plane
kubectl --kubeconfig E:\Sathonate\.cache\smp-qualification.kubeconfig --context kind-smp-qualification get pods -A
```

Start API and console again. Use Operations → View for the saved deployment or resume its saved design. Read progress through the API operation/event endpoints or `kubectl logs job/<job> -n <owned-namespace>` with the explicit kubeconfig/context. Never dump Secrets or unrestricted controller diagnostics into logs.

Use the dashboard's **Delete Local Network** confirmation to remove that network's owned namespace, PVCs, channel CRs and worker binding while retaining PostgreSQL history/design. It refuses cleanup during an active operation. Namespace ownership labels are checked. Cleanup is destructive for that network's ledger. No global Docker prune/delete is required.

For a clean acceptance, delete the previous acceptance network through the product, retain infrastructure, and run the test again. Full infrastructure deletion is a separate explicit operator action, not automatic cleanup. Back up data before intentionally deleting kind or `smp-postgres-data`.

## Cloud prerequisites later (do not execute for local qualification)

An existing authorized AWS account/EKS cluster, region, DNS, storage CSI driver, appropriate IAM/workload identity, scoped Kubernetes RBAC, production secret storage, TLS ingress, Okta issuer/SPA/API audience and tested backups are required. Example access setup for an administrator, using actual approved values:

```powershell
aws configure sso --profile <approved-profile>
aws sso login --profile <approved-profile>
aws sts get-caller-identity --profile <approved-profile>
aws eks update-kubeconfig --profile <approved-profile> --region <region> --name <existing-cluster> --alias <separate-cloud-context>
kubectl --context <separate-cloud-context> auth can-i create deployments -n <approved-namespace>
```

These commands configure access to existing resources; they do not qualify this local worker for cloud execution. A separately implemented, reviewed cloud adapter and explicit user authorization are required before any paid provisioning. Do not rename a cloud context to the local context or bypass local safeguards.
