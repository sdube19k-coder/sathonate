# Controller registry security follow-up — 22 October 2026

Scope: isolated candidate based on Bevel `cd236ccc61d1d159b3dadb4625d08bb5682a8451`. The retained controller, deployment engine, user credentials and 8-deployment network are unchanged. No AWS calls, paid resources or production enablement.

## Design and credential boundaries

The smallest safe CA bootstrap extension uses a same-namespace immutable registry Secret, a reference annotation validated by patched controller code, and a digest-pinned non-root init container. Public registries are empty. Only the Secret name enters chart values; the Secret payload is combined with public configuration inside a memory-backed volume. Independently generated root/TLS registrar credentials are passed to enrollment clients through Secret-mounted files. The candidate supports a deliberately narrow SQLite profile. This is tested controller modification, not an invented upstream CRD field.

Persistent CA databases and CA signing keys remain necessary. The pinned chart also serializes CA private keys into Helm storage Secrets. Secret encryption at rest, backup access and destruction are not qualified. Namespace-level Secret access is still required by the candidate: each namespace must be exclusive to one tenant/network. NetworkPolicy/admission enforcement and per-identity Secret permissions remain unqualified.

## Executed regressions

| Gate | Actual result |
|---|---|
| `go test ./... -count=1 -v`, Docker CA opt-in | PASS: 91 top-level tests, 8 explicit integration skips, zero failures. |
| `go vet ./...` | PASS, exit 0. |
| `TestProtectedCAConfigOnDocker` | PASS, 2.22s; real Docker CA enrollment and reenrollment. |
| PostgreSQL tests, command below | PASS: 23 cases, 2 kind-only skips. Initial parallel-package attempt failed with a concurrent schema migration conflict; serialized rerun passed. |
| `TestEKSWorkerLifecycleOnKind` | PASS, 217.96s: committed transaction `b7e78f94e660ffad7baa7b37838e19f54091c09bce0385286bff113a12276df6`, height 9; cleanup and foreign-resource preservation passed. Local simulation of the EKS target, not AWS evidence. |
| `TestLocalWorkerRegressionOnKind` | PASS, 184.64s: transaction `92364a82bf7df304cad26530cdbf7e42b7d9596ee5bc723f8ab97b6c7fe13e06`, height 9; package total 404.492s. |
| Candidate tests, helper vet, Linux build | PASS: 11 targeted upstream-package tests. API Secret unit tests use a simulated API. |
| Protected Bevel CLI build | PASS against the pinned revision. |
| Python lab safety / evidence redaction | PASS: 4 / 2 tests. |
| Retained user network | PASS: 8/8 ready deployments, read-only check; no retained transaction or mutation. |
| Isolated protected CA bridge | PASS: all 22 real kind checks, including root/TLS enrollment and reenrollment, actual public-resource/Secret-metadata/decoded-Helm/log password inspection, memory-backed runtime configuration and successful enrollment after CA pod restart. Disposable cluster removed. |
| Full replacement Fabric lifecycle | BLOCKED: auxiliary operations/admin identities, channel/chaincode issuance and authority, interrupted reconciliation, recovery and live renewal remain incomplete. |
| Retained-controller private-key log exposure | FAIL, previously confirmed; not rotated or remediated by this candidate. |
| AWS qualification | BLOCKED / NOT RUN. |

CA candidate results are recorded separately in `.logs/controller-registry-kind.json`; no full-network success may be inferred from a CA test or from existing-runner regressions. The first CA attempt failed before resource creation on an incorrect service JSON field. A subsequent attempt completed the init container but failed because Fabric CA needed its config-relative MSP directory on a read-only mount. Further attempts exposed a non-writable database parent created for nested Secret mounts, then a client failure after server-side enrollment because the client updates its configuration. The corrected chart uses a separate writable PVC directory for databases/keystores, memory-backed runtime configuration, and a `Recreate` strategy for SQLite. Client configuration is copied from the read-only Secret to a private RAM workspace. Failed attempts do not count as passing acceptance evidence; their redacted logs and JSON are retained with `*-failure` suffixes.

The passing probe completed at `2026-10-22T17:16:40.683549+00:00`. Tested candidate image: `sathonate/controller-candidate@sha256:b94fe79ab9c9058d2e1abff8d7f7365cb8a4f14ba4d9ffcc7458b100a4232e8c`. Actual CA/client pod image ID: `sha256:09cb0b50ebdb4298e0d69c17d314eb8e96826eeafe2b6a7d14a06b7b6e47fb52`, captured separately in `.logs/controller-ca-image-evidence.json`. This runtime ID differs from Docker's image config ID because the local archive import constructs a platform-specific manifest. The manifest records both kinds of identifier. The earlier 14 security checks precede CA creation: their `noFabricWorkloadCreated` assertion describes the rejected-fixture stage only. The later CA stage creates a real CA. No running password-free peers or orderers are claimed.

CA reenrollment proves issuance of replacement root/TLS client certificates; it does not prove expired-certificate recovery, registrar password rotation, CA signing-key rotation, orderer consenter updates or interrupted init-container recovery. Controller restart was verified against rejected fixtures before CA creation, not a fully running candidate network.

## Reproduction

Use the prerequisites and PowerShell setup in `controller-security-candidate.md`. Commands actually used for this follow-up:

```powershell
python tools/controller-lab/build.py --source E:/Sathonate/dependencies/bevel-operator-fabric --build-dir E:/Sathonate/.cache/controller-security-candidate --output bin/controller-candidate
docker build -f E:/Sathonate/fabric-control-plane/tools/controller-lab/Dockerfile -t sathonate/controller-candidate:lab E:/Sathonate/.cache/controller-security-candidate
python tools/controller-lab/probe.py --kind E:/Sathonate/tools/kind.exe --cache E:/Sathonate/.cache --source E:/Sathonate/dependencies/bevel-operator-fabric --output .logs/controller-registry-kind.json --ca-bootstrap
python tools/kubernetes/build-scoped-cli.py --source E:/Sathonate/dependencies/bevel-operator-fabric --build-dir E:/Sathonate/.cache/bevel-cli-scoped --output bin/kubectl-hlf-scoped
go test ./... -count=1 -v # SMP_CREDENTIAL_DOCKER_E2E=1
go vet ./...
go test -p 1 ./internal/operations ./internal/persistence ./internal/deployment ./internal/maintenance -count=1 -v -timeout 4m
go test ./internal/deployment -run '^(TestEKSWorkerLifecycleOnKind|TestLocalWorkerRegressionOnKind)$' -count=1 -v -timeout 20m
python -m unittest discover -s tools/controller-lab -p 'test_*.py' -v
python -m unittest discover -s tools/engineering -p 'test_*.py' -v
python tools/engineering/evidence.py --output .logs/controller-final-evidence.json
```

PostgreSQL uses `SATHONATE_TEST_DATABASE_URL` injected privately into the process environment, dedicated `fabric_smp_tests` only; never include the value in a command line or evidence. Kind regressions require `SMP_EKS_LOCAL_E2E=1` and `SMP_TEST_KUBECONFIG=E:\Sathonate\.cache\smp-qualification.kubeconfig`. Their existing runner is separate from the candidate. Full Go suite skips are exercised through the opt-in commands above. Serialized database package execution avoids tests concurrently migrating the same dedicated schema.

## Required next gates

1. Resolve historical retained-controller key logging through an approved incident inventory and coordinated certificate/consenter recovery plan. Do not silently rotate a live network.
2. Complete a separately authorized issuer for peer signing/TLS, orderer signing/TLS/admin, peer operations, channel admin and chaincode identities, with supported Secret layouts and independently scoped registration credentials.
3. Qualify the complete candidate network: two peers, three Raft orderers, channel creation/join, TLS chaincode, committed writes/reads, reconciliation, controller/node restarts, interrupted issuance, certificate renewal and recovery. Inspect actual stored CRs, public resources, logs and Helm history. No adoption before this passes.
4. Prove effective namespace exclusivity, admission/CNI isolation, least-privilege controller and channel broker permissions, storage encryption/backups and safe network-specific cleanup.
5. Obtain an explicitly authorized existing EKS target, scoped AWS identity, approved namespaces/AZs, EBS CSI/storage class, digest-approved images, restricted PostgreSQL roles/TLS/backups and administrator/Okta configuration. Run real IAM, storage, placement, isolation and Fabric acceptance on EKS. Local kind and mock results cannot satisfy this gate.

The redacted final manifest records the final source commit, clean-tree status, source/binary hashes, tested candidate digest and allowlisted check results. Raw configuration, connection strings, passwords, keys and logs are excluded. Ignored local evidence is not source-controlled.
