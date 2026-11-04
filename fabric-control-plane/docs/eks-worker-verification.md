# EKS worker verification — 22 October 2026

The EKS qualification adapter is implemented in the existing Go repository. Production cloud execution remains disabled. No AWS environment was contacted and no cloud resources were provisioned during this work.

## Implementation delivered

- `internal/cloud/operator.go`, `config.go`, `package.go`: exact operator-revision translation, network/target digests, approved tenant/namespace bindings, expiring administrator approvals, explicit assumed-role identity and endpoint/CA checks, version/dependency/permission/storage/placement preflight. Operator targets bypass the legacy Ansible/Flux compiler.
- `internal/deployment/eks.go`, `kubernetes.go`, `runner.go`, `evidence.go`: EKS adapter reuses the existing Kubernetes Job observer and Fabric lifecycle. Scoped namespace worker RBAC, named channel reads, channel-resource broker, immutable Job adoption, verified transaction evidence and current workload-health checks. Cleanup retains cloud namespace/PVCs/enrollment Secrets by default.
- `internal/deployment/manager.go`, `recovery.go`, `internal/operations/{store,postgres,recovery}.go`: atomic PostgreSQL job/input submission, single-active scheduler, lock-loss fencing, restart discovery/adoption, idempotency, retries and durable cancellation intent.
- `internal/kubernetesfabric/{worker,runtime,profile}.go`, `cmd/fabric-worker/main.go`: trusted runtime translation for pinned images, storage and required node affinity, namespace-only SDK connection profile, channel broker protocol. Original local profile remains supported.
- `tools/kubernetes/build-scoped-cli.py`, `Dockerfile.eks`, `.dockerignore`: source-built Bevel CLI from commit `cd236ccc61d1d159b3dadb4625d08bb5682a8451`, with reviewed namespace-discovery/internal-address changes. The reference checkout is untouched.
- `cmd/fabric-api/main.go`, `internal/httpapi/{api,cloud,deployment}.go`: separately gated loopback EKS qualification mode, strict cancellation route and operator-plan export. Production mode cannot enable this development qualification runner.
- `cmd/fabric-eks/main.go`, `scripts/write-eks-kubeconfig.ps1`: read-only inspection/preflight utility and explicit role-bound kubeconfig setup. No arbitrary customer commands or inline AWS credentials.
- New tests: cloud operator translation/preflight, EKS binding/RBAC/recovery, broker validation/runtime translation, atomic submission, cancellation request validation and real kind integration/regression cases.
- Setup/examples: `docs/eks-implementation-map.md`, `docs/eks-worker.md`, `docs/cloud-launch.md`, `README.md`, `examples/eks-qualification-network.json`, `examples/eks-operator-targets.template.json`.

The frontend repository was not changed for this engineering task.

## Executed checks

- `go test ./... -count=1` with `SATHONATE_TEST_DATABASE_URL` securely injected for the dedicated `fabric_smp_tests` database: passed. Includes real PostgreSQL persistence/restart, atomic rollback, idempotency, cancellation and terminating only the test executor's own lock session to prove fencing.
- `go vet ./...`: passed.
- Source-built scoped CLI, Linux worker build, Docker image build and kind image load: passed.
- PowerShell parser check for `scripts/write-eks-kubeconfig.ps1`: passed. Actual AWS setup command execution is unverified.
- `fabric-eks.exe -action inspect ...` against the supplied templates: passed, reported `awsVerified=false` and production execution disabled, made no cloud calls.
- API startup with `-deployment-mode real`: rejected as expected. Unconfigured `-deployment-mode eks-qualification`: rejected as expected.

Full Go output: `E:/Sathonate/fabric-control-plane/.logs/eks-go-tests-20261022.log`.

## Real local Fabric evidence

The final scoped-worker test uses the EKS-style manifest, actual namespace RBAC, the pinned runtime images, required placement, channel brokering and real Fabric transactions on kind. It explicitly denies unfiltered cluster channel listing, channel creation by the worker and Secret reads in the default namespace. The controller detaches while the Job is active, reattaches to that same Job UID and subsequently replays completed evidence without creating a replacement. This is local Kubernetes evidence, not EKS/IAM/EBS evidence.

Final scoped operation: `op_b30ca94598a2e5f9fd5a282708f6bc1b`.
Transaction: `5b1c4a376bc98bc61c6a65b01f755046bc63e75945571bc27f194fe7a9a0f97e`.
Both peers reached height 9. The final scoped test passed in 168.35 seconds, including cleanup. Its namespace `smp-n-8c9175f15f118a92` was confirmed absent afterward.

Command: with `SMP_EKS_LOCAL_E2E=1` and explicit `SMP_TEST_KUBECONFIG`, `go test ./internal/deployment -run '^TestEKSWorkerLifecycleOnKind$' -count=1 -v -timeout 18m`.
Log: `E:/Sathonate/fabric-control-plane/.logs/eks-scoped-final-20261022.log`.

The independent original local-profile regression passed in 159.62 seconds, including cleanup. It used the same newly built worker image without the EKS runtime and verified transaction `1769b9165815478d03ba822f2ed646167948bcc411cdcccc639013908de891c9`, with both peers at height 9. Command: `go test ./internal/deployment -run '^TestLocalWorkerRegressionOnKind$' -count=1 -v -timeout 18m` with the same local-test environment. Its final result is in `E:/Sathonate/fabric-control-plane/.logs/local-regression-final-20261022.log`.

## Failures found and corrected

Early real test runs failed and are preserved in diagnostic logs. Corrections included the CA `version` versus peer/orderer `tag` image fields; upstream CLI cluster-wide CA/node discovery; the Bevel CRD/Helm nodeSelector mismatch (required affinity is used instead); temporary empty FAILED status during Helm startup; the name-restricted list permission needed by kubectl wait; and safe adoption of existing channel CRs during completed-Job replay instead of reapplying them. A pre-fix run was interrupted after its known placement failure; only its explicitly verified test namespace/roles were cleaned up. No retained user network was deleted.

## Remaining qualification prerequisites and limits

Supply an explicitly authorized existing EKS Kubernetes 1.34 environment, company assumed-role profile and EKS access mapping, three approved AZ placements, an exclusive labelled namespace with reviewed isolation/quota policies, encrypted delayed-binding EBS CSI storage, the pinned operator/CRDs, accessible pinned worker/smoke images, a qualification PostgreSQL database and a target-digest approval. Exact Windows commands and configuration examples are in `docs/eks-worker.md`.

Actual EKS, AWS IAM access, registry pulls, EBS attachment/recovery, three-AZ failure recovery, network-policy enforcement, backup restoration, certificate renewal and live Okta enterprise isolation remain unverified. The worker supports the bounded Fabric 2.5.15 / CA 1.5.15 / two-organization / two-peer / three-Raft-orderer / one-channel smoke profile; it does not claim arbitrary topology or customer chaincode support. This is a single-active scheduler, not a horizontally distributed worker fleet. Production remains gated.

## Running development services

The UI on `http://127.0.0.1:5173/` and API on port 8080 are running in local Kubernetes mode. The retained operation `op_9ac07bd1f386c3a692efb5a02082bca8` returned `ready=true`; production cloud execution remains false. New EKS qualification binaries are `bin/fabric-api-eks.exe` and `bin/fabric-eks.exe`; qualification has not been enabled against any AWS target.
