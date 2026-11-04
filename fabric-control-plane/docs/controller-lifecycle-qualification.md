# Milestone 2: secure controller lifecycle qualification

This follows the completed CA registry prerequisite at `ce1600e7103e19069a6fa7dc8679af4eb315d409`. The working engine/controller and retained namespace `smp-n-968a212068be7d07` are not modified. All candidate tests use randomly named, separate kind clusters. AWS qualification and production execution remain disabled.

## Identity inventory and implementation map

| Identity | Issuance and delivery |
|---|---|
| Root/TLS CA registrars, two organizations | Four different generated passwords in immutable registry Secrets; init configuration and registrar MSPs in RAM. Registration cannot delegate, revoke, issue intermediates or administer affiliations. |
| Each peer signing identity | Separate CA registration, type peer; versioned external certificate/key/root Secret. |
| Each peer TLS server identity | Separate TLS CA registration, SANs covering approved service DNS; versioned external Secret. |
| Each peer operations TLS identity | Separate TLS CA client registration; validated conventional `-tls-ops` Secret and owned `-tlsrootcert`. |
| Each orderer signing and TLS identity | Separate registrations and external Secrets for each role and orderer. Three Raft orderers, Fabric 2.5.15. |
| Each orderer admin TLS server | Separate TLS CA client identity; owned `-admin` Secret with pinned supported key layout and same-CA client trust. |
| Organization/channel administrators | One admin signing identity per organization for channel joins, plus a separate TLS admin client per organization for participation API calls. |
| Chaincode lifecycle administrators | Separate admin signing identities for installation, approval and commit in each organization. |
| Transaction clients | Separate client signing identity per organization; used to verify sample writes/reads. |
| Chaincode TLS server | Separate TLS CA identity, validated service SAN and external Secret. |

The lab issues 28 different enrollment credentials: four registrars and 24 role-specific registrations. Enrollment and registration use protected configuration files in the issuer pod's memory-backed workspace. No enrollment password is passed through kernel argv or written to ordinary host artifacts. The protected pinned CLI receives arguments through stdin. Public CRs contain certificate references and empty enrollment-password fields. The unused LevelDB profile's CouchDB password is cleared.

The peer validator explicitly rejects CouchDB and PostgreSQL state databases in this isolated candidate. Clearing a database password must never accidentally admit an unqualified database profile. This is a fixed two-organization, one-peer-per-organization, three-Raft-orderer qualification; it does not qualify arbitrary designs, Solo/BFT or production adoption.

`tools/controller-lab/material.go.tmpl` validates auxiliary trust/ownership and moves private chart material directly into owned Kubernetes Secrets before chart serialization. The patched charts retain supported Secret names but render no private-key Secret bodies into Helm history. Only public certificates contribute rollout checksums. `channel.go.tmpl` reuses the pinned Bevel application-channel block builder with fixed lab topology; the harness calls Fabric participation APIs and peer commands, without cluster-scoped channel controllers or permissions.

`lifecycle.py` drives the actual network, inspects runtime objects/release storage/logs and performs fault/recovery and renewal checks. The issuer has read-only namespace-local Fabric API access and cannot read Kubernetes Secrets. Only its tools container receives a narrowly scoped API token; the CA client and network workloads need none. The controller retains namespace-level Secret privileges for its cache/Helm workflow. Namespace exclusivity remains mandatory; this does not establish per-identity Secret isolation or effective CNI/admission enforcement.

The two issuer containers are equally trusted: they share a process namespace and memory workspace for the argv audit. A token mounted only into the tools container is not a separate security-principal boundary. The client should not be treated as an untrusted sidecar. The process audit samples actual enrollment commands; fixed source-controlled commands and stdin/config-file delivery provide the complementary argument-construction guarantee.

Runtime log scans run before subsequent restarts, after restart/recovery/renewal transactions, and at final inspection. Unreadable logs block verification. The missing-reference test requires a new rejection mentioning the affected peer after the patch and an unchanged workload template, rather than accepting an old fixture error. The outage test queues a certificate-reference update while the candidate has zero pods, then verifies reconciliation, the live leaf certificate and committed transactions after restart. It does not test power loss halfway through a Helm/Secret write.

## Qualification results

The authoritative candidate result is `.logs/controller-lifecycle-kind.json`; final redacted commit/image evidence is `.logs/controller-final-evidence.json`. Implementation is not a passing runtime result. A run that stops at channel creation is not a full deployment.

Completed 23 October 2026 at `2026-10-23T04:49:41.130586+00:00` (UTC), process exit 0, elapsed 535.25s including disposable setup and cleanup. All nine stages and all 42 checks passed. Tested image: `sathonate/controller-candidate@sha256:a467cb2986f33d870477730b8ef3b0c56991280b14cf426091b2d28fa739b03a`. Only `smp-qualification` remains in `kind get clusters` after cleanup.

| Gate | Final result |
|---|---|
| CA bootstrap, all signing/TLS/auxiliary identities, three orderers, two peers | PASS, real disposable kind. 28 distinct enrollment credentials. |
| Channel participation/join, chaincode package/install/approve/commit | PASS, actual Fabric APIs and peer binary. |
| Committed writes and reads on both peers | PASS after initial deployment, restart, missing-reference recovery, TLS renewal and renewed-certificate restart. |
| Missing certificate reference | PASS, new rejection for the affected peer, unchanged workload template, successful reference restoration and transactions. |
| Controller outage during certificate update | PASS, desired reference queued with zero controller pods; live renewed certificate and transactions verified after recovery. This is not an in-flight Helm crash test. |
| Actual CR/ConfigMap/Pod/Deployment credential inspection | PASS, empty enrollment-password fields and no known passwords/private keys, including encoded key checks. Template inspection alone was not used. |
| Decoded Helm values/manifests/history | PASS, private material absent for the actual candidate releases. Original history is unchanged. |
| Logs, process argv, workload tokens, namespace denial and cleanup | PASS, actual sampled runtime inspections and API authorization failures. Whole disposable cluster removed. |
| Full Go suite with Docker CA opt-in | PASS, 91 top-level tests; 8 explicit integration skips, separately opted in below. Docker CA enrollment/reenrollment PASS, 5.66s. |
| PostgreSQL integration, serialized dedicated test database | PASS, 23 top-level cases; 2 kind-only skips. Restricted-role operation/deployment RLS 0.08s. |
| Root Go vet | PASS, exit 0. |
| Candidate targeted Go tests / vet / Linux build | PASS, 14 tests, helper/controller/main vet and Linux build. Fake API unit tests are not AWS evidence. |
| Protected pinned CLI build | PASS, source pin unchanged. |
| Python lab / evidence redaction | PASS, 10 lab tests and 3 evidence tests, no skips. |
| Existing scoped worker real kind regression | PASS, 183.58s; both peers height 9; transaction `935a96c9e01473ca00630bc011b729b3080c083482abfc807df1810ef9b79718`. |
| Existing local worker real kind regression | PASS, 183.30s; both peers height 9; transaction `62f59c20206ced512f35ee356398a65e1057aac6b638da8ea1a700eb9f9d606c`. Combined package 368.638s, cleanup included. |
| Retained network | PASS, read-only 8/8 deployment readiness after regressions; no retained transactions, resource patches, controller rollouts or credential rotation requested. |
| Original/retained controller private-key logging | FAIL, previously confirmed exposure persists; not remediated by this candidate or by ordinary-runner regression passes. No raw key/log contents exported. |
| Other identity renewal, expired recovery, CA/root rotation and partial-write crash recovery | BLOCKED / NOT VERIFIED as detailed below. Peer TLS success is not evidence for those operations. |
| Effective CNI/admission isolation and AWS EKS | BLOCKED / NOT RUN. No cloud resources created and no production execution enabled. |
| Milestone 2 fully complete / production adoption | NO. The fixed local candidate lifecycle passes; remaining security/promotion gates below are unresolved. |

Earlier failed attempts are retained as redacted lab artifacts: ordering pull-policy schema, participation client escaping, missing local MSP NodeOU configuration, and chaincode server-port schema. Each failed cluster was removed. Those attempts are not counted as passes. The NodeOU and chaincode schema failures have regression coverage. A prior full 40-check run also passed; the final 42-check run adds the outage qualification and stricter rejection evidence.

Docker Desktop's Linux engine was unavailable initially. Launching Desktop restored the local kind environment; the existing PostgreSQL container was started without changing its data. Tests use only `fabric_smp_tests`, not the user database. A transient image-build failure during engine startup was rerun successfully. No retained Kubernetes rollout/scale/patch command or live credential rotation was issued.

Changed implementation: `build.py` pins and patches the candidate; `material.go.tmpl` keeps private chart material in owned Secrets; `registry.go.tmpl` refuses unqualified CA material paths; `main.go.tmpl` and `channel.go.tmpl` provide bounded public channel genesis generation; `lifecycle.py` and its tests qualify the disposable path; `evidence.py` and its tests export allowlisted, redacted results. The engineering report and security matrix link this qualification. Existing application runners are unchanged.

The final source commit is the manifest's `sourceRevision`, generated after the reviewed commit; `sourceClean`, `controllerCandidateBuild.currentPatchInputsMatch` and `currentBinaryMatches` must be true. Docker image IDs are config digests; `testedImage` and `runtimeImageDigests` are the actual kind runtime references, not a claim of registry approval for EKS. Saved JSON alone is not a CI exit-status check: accept a result only alongside its successful command exit, timestamps and matching source/build evidence.

## Exact reproduction commands

Use the existing Windows prerequisites and Go cache settings in `controller-security-candidate.md`. Docker must have Linux containers running, cached kind v1.34.0, Fabric 2.5.15 peer/orderer images, Fabric CA 1.5.15, and the existing worker/smoke images. The original local profile is unchanged. Verified public-image transport archives may be cached outside the repository, indexed by Docker image config ID and checked by SHA-256 before reuse; they contain no customer configuration.

```powershell
Set-Location E:/Sathonate/fabric-control-plane
$ErrorActionPreference='Stop'
$PSNativeCommandUseErrorActionPreference=$true
$env:GOCACHE='E:/Sathonate/.cache/go-build'
$env:GOPATH='E:/Sathonate/.cache/go'
$env:GOPROXY='off'
python tools/controller-lab/build.py --source E:/Sathonate/dependencies/bevel-operator-fabric --build-dir E:/Sathonate/.cache/controller-security-candidate --output bin/controller-candidate
docker build -f E:/Sathonate/fabric-control-plane/tools/controller-lab/Dockerfile -t sathonate/controller-candidate:lab E:/Sathonate/.cache/controller-security-candidate
python tools/controller-lab/lifecycle.py --kind E:/Sathonate/tools/kind.exe --cache E:/Sathonate/.cache --source E:/Sathonate/dependencies/bevel-operator-fabric --output .logs/controller-lifecycle-kind.json
$env:SMP_CREDENTIAL_DOCKER_E2E='1'
go test ./... -count=1 -v
go vet ./...
Remove-Item Env:SMP_CREDENTIAL_DOCKER_E2E
# Inject SATHONATE_TEST_DATABASE_URL privately, dedicated fabric_smp_tests only.
go test -p 1 ./internal/operations ./internal/persistence ./internal/deployment ./internal/maintenance -count=1 -v -timeout 4m
$env:SMP_EKS_LOCAL_E2E='1'
$env:SMP_TEST_KUBECONFIG='E:/Sathonate/.cache/smp-qualification.kubeconfig'
go test ./internal/deployment -run '^(TestEKSWorkerLifecycleOnKind|TestLocalWorkerRegressionOnKind)$' -count=1 -v -timeout 20m
Remove-Item Env:SMP_EKS_LOCAL_E2E,Env:SMP_TEST_KUBECONFIG
python tools/kubernetes/build-scoped-cli.py --source E:/Sathonate/dependencies/bevel-operator-fabric --build-dir E:/Sathonate/.cache/bevel-cli-scoped --output bin/kubectl-hlf-scoped
python -m unittest discover -s tools/controller-lab -p 'test_*.py' -v
python -m unittest discover -s tools/engineering -p 'test_*.py' -v
Push-Location E:/Sathonate/.cache/controller-security-candidate
go vet . ./controllers/smpsecurity ./controllers/peer ./controllers/ordnode ./controllers/ca ./controllers/chaincode/deploy
Pop-Location
python tools/engineering/evidence.py --output .logs/controller-final-evidence.json
```

Use at least the existing local resource allocation (Docker currently approximately 5.8 GiB / 20 CPUs) and enough disk space for a second kind node and public image archives. Run full-network regressions sequentially to avoid competing with a candidate network for this memory allocation. PostgreSQL cases run serially because packages share the dedicated test schema.

The verified Windows toolchain is Go 1.26.7, Python 3.12.10 and kind 0.33.0, using `kindest/node:v1.34.0`. Python requires PyYAML on the host; the existing worker image includes cryptography for matching reenrolled certificates to the correct private key. `GOPROXY=off` assumes the existing module cache is populated. It is not a command for downloading dependencies into a clean machine. Existing setup instructions remain in `controller-security-candidate.md` and the local environment documentation.

## Remaining security boundaries and AWS prerequisites

The original pinned controller's private-key logging exposure is a confirmed retained-environment finding. No raw sensitive log contents are reproduced. The candidate removes that formatter, discards legacy logrus output and validates material before legacy parsing; runtime sampling must still pass after reconciliation errors. This does not revoke historical keys, sanitize log backups or change the retained controller. Owner-approved incident assessment and coordinated rotation/recovery are required before using that environment for customers.

Kubernetes Secret values are required private storage, not encryption. CA signing keys and databases also require persistent private storage. EKS needs approved etcd/Secret encryption, EBS encryption and backup access controls. Historical Helm releases from the original controller are not migrated or deleted. Candidate-only empty Helm key fields do not remediate retained history.

Peer TLS renewal uses certificate-authenticated reenrollment, a new immutable external Secret, a reference update and public-certificate rollout checksum. Automatic expired-certificate recovery, issuer/CA signing-key rotation and Raft consenter TLS replacement are separate operations: an orderer certificate is pinned into the channel's consenter configuration, so replacing a Secret alone cannot safely rotate it. A coordinated channel configuration update must be qualified before those operations can be declared supported. Separate admin-client trust roots and external CA key-reference modes are outside the candidate's tested profile and must fail closed rather than use password fallback.

The CA registry Secret is deliberately immutable. Changing its reference does not rotate an identity already stored in Fabric CA's database; an approved CA identity modification and staged registry/configuration update would need separate qualification. Peer signing/operations, orderer admin, organization administrator and chaincode certificate replacement are not qualified by the peer TLS test. Use a separately qualified issuer/broker and identity-specific rollout procedures before offering those operations.

Managed chart Secrets are updated before Helm reconciles dependent public configuration. Kubernetes does not offer a transaction spanning those writes. Recovery from a missing reference and a controller outage is tested separately from a crash halfway through a material/configuration update; the latter still needs fault injection and potentially versioned immutable Secret mounts plus an atomic pod-template switch. CA database restore, encrypted backups, credential revocation and original release/log history cleanup are also unqualified. Disposable whole-cluster cleanup proves ownership-bounded lab teardown, not every production per-resource finalizer or disaster-recovery path.

Before Milestone 3: complete all Milestone 2 acceptance evidence; settle remaining lifecycle limits and incident handling; obtain explicit approval for an existing EKS target/account/region/role and exclusive tenant/network namespace; qualify minimum controller/issuer RBAC, quotas, admission and effective CNI isolation; approve immutable image digests and registry access; configure encrypted EBS CSI storage, approved AZ placement, restricted PostgreSQL roles/TLS/backups and administrator/Okta configuration; run actual EKS Fabric acceptance, IAM/storage/placement/isolation and bounded cleanup tests. Local kind and simulated AWS tests never satisfy AWS qualification. No cloud resources or credentials are assumed.

The API/local/EKS runners do not automatically select this lab controller or its Python issuer. Production adoption still requires an explicitly versioned candidate profile, a protected and tenant-authorized issuer/broker, approved trust roots, durable rotation/recovery operations and qualification through the real runner. Do not reuse any identities whose private keys appeared in historical original-controller logs. A fresh target can use new identities, while assessment/rotation of the retained environment remains an owner-approved task. No default operator image or production flag is changed here.

Prioritized remaining gates:

1. Keep the retained exposure contained and agree an owner-approved incident/migration/rotation plan; never reuse exposed keys. Retained changes are outside this authorization.
2. Qualify atomic signing-material updates and interruption/rollback between Kubernetes and Helm writes, plus the missing identity-specific renewal/revocation/expired-certificate and CA restore procedures. The smallest safe next implementation is an isolated versioned certificate/key bundle and pod-template switch with failure injection, preserving this passing candidate as the baseline.
3. Approve the production issuer/broker and runner integration, exclusive namespace/tenant trust model, narrowly scoped controller roles, effective CNI/admission/quotas and encrypted Secret/PVC/backup handling. The local Role still reads Secrets across its own namespace; it is not safe for independent tenants sharing that namespace.
4. Supply an explicitly authorized existing EKS cluster/account/region, approved target reference and namespace, locally configured AWS role/workload identity and Kubernetes access. IAM must be limited to the approved identity/target, with no cloud-provisioning permissions inferred from a feature flag. The cluster still needs to be provided; none is assumed to exist.
5. Approve immutable registry images and pull access, encrypted EBS CSI storage and approved availability-zone placement, PostgreSQL TLS/restricted roles/RLS/backups, administrator/Okta settings and cloud-runner recovery/cleanup limits. Run the readiness verifier and real EKS Fabric acceptance only after explicit authorization. Kind and mocked AWS API tests never satisfy this gate.
