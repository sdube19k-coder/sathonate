# Controller security candidate — current qualification

The previously verified follow-up was reviewed and committed as `4457e8e894f53fb7b5e0cad4e7cdee1aa8c6ce9e`. Fourteen staged files were checked; no credential patterns or generated/private artifacts were found. Baseline `e378cbf66fa74f3c533231830ec9225ce3aaa353` remains in history. Existing controller, worker engine and retained resources are preserved.

## Candidate scope

`tools/controller-lab/build.py` archives exact Bevel revision `cd236ccc61d1d159b3dadb4625d08bb5682a8451` into a separately marked directory, applies source-drift-checked patches, runs targeted Go tests and builds a Linux candidate. The reference checkout is never modified. Source patch inputs, patched source and binary hashes are embedded in its container as `/source-evidence.json`. It is not a production image or supported replacement.

Changes in this isolated archive:

- Explicit namespace-scoped manager cache; registers only CA, peer, orderer and chaincode deployment controllers. No cluster-channel, identity, UI or RBAC bootstrap controllers. Chaincode templates and their watch are explicitly disabled to avoid an unnecessary informer and permissions.
- Peer/orderer require complete same-namespace external signing and TLS references, reject inline secrets, Vault mixtures and enrollment fallback, validate actual ECDSA cert/key/trust and Secret ownership labels before crypto reads.
- Missing peer operations TLS or orderer admin TLS material fails closed instead of reenrolling from empty or retained passwords. Existing auxiliary Secret material is not fully qualified for automatic renewal; no production claim is made.
- TLS chaincode uses the already defined `TLSComponent.external` reference. It validates the Secret and service DNS, mounts the external Secret using existing key names, does not reenroll, and does not delete the issuer-owned Secret during finalization.
- Removes the orderer private-key logging statement. The lab manager discards upstream logrus diagnostics; this prevents that logging channel but loses actionable upstream diagnostics. Controller-runtime errors remain and must be qualified/redacted before production use. This is not a complete logging remediation for the retained controller.
- CA bootstrap fails closed with a durable FAILED status. A protected registry configuration bridge is **not implemented**. The candidate therefore cannot yet deploy a functioning Fabric network.

No new CRD fields are invented. This deliberately limited candidate tests stronger boundaries without pretending that the full migration is complete.

## Exact technical blockers and alternatives

1. **CA bootstrap.** Pinned `FabricCA.spec.ca.registry.identities[].pass` and TLS CA equivalent map into Helm values and chart ConfigMaps (`configmap--config.yaml`, `configmap--config-tls.yaml`). The existing CA signing-key Secret reference does not supply registry credentials. Resolving a password in the controller and copying it into Helm merely relocates the leak. Clearing a CR after bootstrap leaves Helm history/configuration/audit exposure. The candidate refuses this route.
2. **Auxiliary identities.** External signing/TLS support alone does not cover peer operations TLS and orderer admin TLS. Their upstream missing-Secret/recovery branches enroll again. Candidate denies that fallback; a versioned pre-enrollment and recovery design must supply and validate these auxiliary identities before startup.
3. **Channel authority.** Upstream channel resources/controllers have cluster scope. Registering them in a nominally namespaced controller would reintroduce broad watches/Secret access. The candidate excludes them. A dedicated broker must authorize every participating namespace and use pre-enrolled wallet Secrets without giving the node controller cluster-wide Secret reads.
4. **Renewal/recovery.** Unit tests prove new valid material is read and bad trust/key/SAN is refused. They do not prove TLS consenter rotation, peer/channel admin recovery or cert replacement during live reconciliation. Live rotation is disabled; current user credentials were not rotated.
5. **Helm/private keys.** Existing node charts/controller rendering copy certificate private keys into Helm release material. Eliminating enrollment passwords is not equivalent to eliminating all private material from persistent artifacts. A qualified mount-based chart/controller redesign and backup/audit policy are still needed.

Viable next designs: (a) explicitly reviewed same-namespace CA registry Secret reference plus runtime-mounted registry configuration, with no secret payload in Helm values/manifests/ConfigMaps; pre-enroll all node/admin/operations/chaincode identities in a separate least-privileged issuer; (b) external enterprise CA/Vault PKI with separately qualified issuance and rotation, retaining Fabric CA compatibility where required; (c) dedicated tenant clusters as an interim containment boundary, with the existing password-bearing deployment explicitly labelled development-only. None of these is enabled by this change. No static dummy password, insecure fallback, raw private configuration annotation or untested CR field is used to force a passing network.

## Protected CA registry candidate follow-up

The isolated candidate now adds an explicitly patched and source-drift-checked CA registry bridge. This is a candidate controller extension, not an upstream-supported CRD field. The annotation `sathonate.io/ca-registry-secret` references an immutable, namespace-owned Secret containing `registry.json`. The real FabricCA registries must be empty. Root and TLS registrars have separately generated credentials and narrowly restricted registration attributes; neither can delegate, revoke, administer affiliations or issue intermediate CAs. This does not change the working deployment engine.

The controller validates the Secret but passes only its name and a digest-pinned init image to Helm. A non-root init container combines public configuration with the mounted Secret into a memory-backed emptyDir. Fabric CA needs write access there to create its default MSP directory. Server and enrollment pods have no service-account token; enrollment client configurations, private diagnostics and client MSP files stay in Secret mounts or memory volumes. No registry passwords are put in argv, public CR fields, ConfigMaps or chart values. Server CA keys and SQLite databases still require persistent storage; Kubernetes Secrets and Helm history still contain CA private-key material under the pinned chart. This change does not eliminate that exposure or qualify encryption at rest, backups or incident response.

Only the fixed SQLite profile is supported by this prototype. Registry mutation is rejected, and replacing a Secret reference is not a qualified registrar rotation procedure: existing CA database identities are not automatically rotated. Interrupted init-container writes, database recovery, expiry/revocation and full node identity issuance remain unqualified. The original registry-delivery blocker described above is historical; real follow-up results and remaining lifecycle blockers are recorded in `docs/controller-registry-followup.md`.

Run the disposable CA test with `--ca-bootstrap` in addition to the reproduction command below. It starts a real CA, enrolls and reenrolls root/TLS identities, inspects actual public API resources and decoded Helm history privately, and restarts the CA before another enrollment. It creates and removes its own cluster only. Image archive normalization strips redundant OCI index metadata during local Docker-to-kind transport without changing config/layer bytes; this is not an approved cloud image publishing workflow.

## Critical retained-controller finding

The pinned orderer source logs `signEncodedPK` and `tlsEncodedPK`. A read-only sample of the existing controller's last 1000 log lines contained private-key markers. Only booleans and the sample window were exported in `.logs/controller-upstream-log-audit.json`; raw logs/private keys were never emitted or stored by this audit. **Credential exposure is not resolved.** Restrict access to operator logs and treat historical aggregation/backups as potentially sensitive. An owner-approved incident response must assess which identities were exposed and coordinate rotation/revocation and ledger/consenter recovery. No automatic rotation, log deletion or retained-controller change was performed under this task.

## Disposable security probe

`tools/controller-lab/probe.py` creates a new randomly named kind cluster, uses a new restricted kubeconfig with explicit context/provider checks, installs pinned schemas, and runs the candidate with a Role/RoleBinding. It never reuses the retained `smp-qualification` cluster. The Role has no cluster grants, no nodes/namespaces/RBAC permissions, no service-account creation, and only the four supported namespaced Fabric kinds. Namespace Secret list/watch remains necessary for Helm/cache in this prototype: a single namespace must belong to exactly one tenant/network. This is not per-identity Secret isolation and does not prove admission/CNI enforcement.

Actual GET/list/create/delete negative probes require Forbidden responses; NotFound does not count. The foreign Secret is checked for preservation. Password-free malformed/rejected schema fixtures are inspected from the API; this must never be described as password-free operational deployment evidence. Whole disposable-cluster removal is guarded by exact kind provider identity and does not touch the retained network. Local lab cleanup is not production data destruction.

## Reproduction (PowerShell)

Prerequisites: existing Docker Linux engine, kind/node image v1.34.0, kubectl, Go, Python 3.12 with PyYAML, pinned Bevel checkout, existing worker/smoke images for the original regressions; enough memory for an additional disposable kind control plane. No AWS setup is used.

```powershell
$ErrorActionPreference='Stop'
$PSNativeCommandUseErrorActionPreference=$true
Set-Location E:/Sathonate/fabric-control-plane
$env:GOCACHE='E:\Sathonate\.cache\go-build'
$env:GOPATH='E:\Sathonate\.cache\go'
$env:GOPROXY='off' # use the approved Go proxy to fill missing modules if necessary
python tools/controller-lab/build.py --source E:/Sathonate/dependencies/bevel-operator-fabric --build-dir E:/Sathonate/.cache/controller-security-candidate --output bin/controller-candidate
docker build -f E:/Sathonate/fabric-control-plane/tools/controller-lab/Dockerfile -t sathonate/controller-candidate:lab E:/Sathonate/.cache/controller-security-candidate
python tools/controller-lab/probe.py --kind E:/Sathonate/tools/kind.exe --cache E:/Sathonate/.cache --source E:/Sathonate/dependencies/bevel-operator-fabric --output .logs/controller-candidate-kind.json
python -m unittest discover -s tools/controller-lab -p 'test_*.py' -v
python -m unittest discover -s tools/engineering -p 'test_*.py' -v
$env:SMP_CREDENTIAL_DOCKER_E2E='1'
go test ./... -count=1 -v
go vet ./...
Remove-Item Env:SMP_CREDENTIAL_DOCKER_E2E
# Inject SATHONATE_TEST_DATABASE_URL from protected configuration, dedicated test DB only.
go test -p 1 ./internal/operations ./internal/persistence ./internal/deployment ./internal/maintenance -count=1 -v -timeout 4m
$env:SMP_EKS_LOCAL_E2E='1'
$env:SMP_TEST_KUBECONFIG='E:\Sathonate\.cache\smp-qualification.kubeconfig'
go test ./internal/deployment -run '^(TestEKSWorkerLifecycleOnKind|TestLocalWorkerRegressionOnKind)$' -count=1 -v -timeout 20m
Remove-Item Env:SMP_EKS_LOCAL_E2E,Env:SMP_TEST_KUBECONFIG
python tools/engineering/evidence.py --output .logs/controller-final-evidence.json
```

Do not run schema tests against a customer database. Test DB password is passed in process environment from a protected local file, never printed or passed in argv. Candidate test API servers simulate Kubernetes Secret reads; real negative authorization uses disposable kind; original runner transactions use real kind Fabric. AWS/IAM/EBS/AZ/Okta qualification is not exercised.

## Acceptance boundary

A complete replacement must pass initial CA startup, peer/orderer/operations/admin identity provisioning, channel joins, TLS chaincode, committed transaction reads from both peers, restart, interrupted reconciliation, controlled certificate renewal/recovery, actual stored-CR/ConfigMap/Helm inspection and negative tenant/controller authorization. The candidate currently cannot meet that boundary and must not be adopted. Final results are in the engineering report and `.logs/controller-final-evidence.json`.


Failed-attempt accounting: the first lab probe failed on an absent optional orderer Secret fixture (and Windows subprocess decoding), then corrected both without changing acceptance requirements. Subsequent probes failed waiting for durable refusal observations. Investigation found an upstream chaincode template informer preventing cache synchronization under the intentionally restricted Role. The candidate removes that unused watch and rejects templates instead of broadening privileges. These failed runs are preserved as attempt evidence and do not count as completed Fabric tests. The upstream private-key logging finding remains a separate retained-controller risk.


Further failed-probe accounting: missing required non-null CA conditions caused the first failure-status update to be rejected; candidate now initializes the conditions array. A later probe completed 13 security checks and restart but failed an extra annotation mutation; that is not counted as a passed interrupted-reconciliation test. The final probe checks post-restart fail-closed state only. Interrupted mutation/recovery remains NOT VERIFIED. Acceptance requirements were not reduced to claim a working network; the full candidate Fabric lifecycle remains BLOCKED.

The post-restart probe also exposed an incorrect singular kubectl resource name (`fabricca`); the pinned CRD is queried using `fabriccas`. This probe-command failure was corrected; prior failed attempts remain failed evidence.
