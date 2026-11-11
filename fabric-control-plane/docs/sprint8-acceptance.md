# Sprint 8 acceptance

**Overall PARTIAL.** Baseline `cb4733b`. Sprint 6 and Sprint 7 remain PARTIAL. This run used local builds, controlled fixtures and Helm rendering. It made no Kubernetes cluster or cloud calls.

| Gate | Software evidence | Live status |
| --- | --- | --- |
| Customer control-plane packaging and HTTPS routing | **PASS**. Helm 3.19.0 strict lint and YAML assertions for Ingress and external gateway profiles (10 and 5 resources). Five invalid configurations rejected. The console server fixture verified the SPA, callback, asset paths and API route separation. | **BLOCKED**. Requires a customer-owned namespace, real ingress/gateway and TLS, independently provisioned PostgreSQL, Okta and published image digests. Fabric execution remains disabled in the chart. |
| Dynamic Fabric storage selection | **PASS**. A topology fixture uses separate approved classes for Fabric nodes and CA storage. Unknown, unencrypted, immediate-binding and undersized classes are rejected. | **BLOCKED**. The customer target adapter must bind the class inventory to the intended cluster and validate the CSI provisioner, encryption, PVC ownership, zone affinity, recovery and actual transaction continuity. |
| Certificate lifecycle | **PARTIAL**. Local TLS server verification with explicit roots, hostname, server EKU and renewal horizon passed. The fixture rejects expired, foreign, wrong-name, client-only, key-bearing and missing-root inputs. | **BLOCKED**. Requires live inventory and durable certificate rollout wiring. TLS leaf verification covers public server chains. MSP signing, issuer changes, revocation, historical key rotation and live workload rollover have separate acceptance requirements. |
| Prometheus metrics and alerts | **PASS** for bounded HTTP metrics, redacted readiness and rendered scrape/alert resources. | **BLOCKED**. Prometheus Operator discovery, CNI access isolation, rule evaluation and Alertmanager delivery require a customer target. Fabric ledger, Raft and certificate-expiry exporter coverage remains open. |
| Peer, Raft orderer and CA upgrade safeguards | **PARTIAL**. Fixture verifies server-bound tenant/project, target and topology digests, fresh channel/voter evidence, quorum, a mutation fence, reviewed image transitions and paused CA enrollment. An image-only compare-and-swap patch passed a mocked Kubernetes endpoint, with foreign UID, stale revision, wrong container and changed image cases rejected. | **BLOCKED**. Requires a trusted desired-state owner, durable approval/operation journal and cross-replica fencing implementation, actual per-channel endorsement and continuity checks, controlled recovery, and post-rollout ledger/transaction/TLS/storage verification. |
| Load and soak | **PASS** for local harness behavior. At 10 requests/second and four workers, the 60-second 204 fixture recorded 600/600 successful probes within 500 ms and zero drops (59.891 seconds measured). The 503 fixture correctly failed with 0/20 successful probes. | **BLOCKED**. This is readiness traffic. Production API throughput, sustained SLOs and Fabric transaction performance need a representative environment and a committed-transaction workload with ledger verification. |
| Regression suites | **PASS**. Fresh Go tests and vet, console 31 tests and production bundle build, the two Python soak tests, and Linux amd64 builds of both image entry points. The Vite build reported a 521 kB minified JS chunk warning. | The standalone bearer-mode startup probe stopped at the PostgreSQL project-isolation requirement. No temporary database was started for this probe. Browser authentication and customer Kubernetes qualification remain external dependencies. |

## Customer deployment profile

[Chart and values](../deploy/charts/sathonate/values.yaml) · [Ingress profile](../deploy/charts/sathonate/profiles/customer-ingress.yaml) · [External gateway overlay](../deploy/charts/sathonate/profiles/customer-gateway.yaml)

1. Build the Linux `fabric-api` and `fabric-console-server` binaries with `CGO_ENABLED=0`, build the console with its production configuration, and package both using [the image recipes](../deploy/images). Set `apiImage` and `consoleImage` to published digests. The API runtime base must provide a trusted CA store. Set `imagePullSecrets` to existing same-namespace registry Secret names for private images. The image reference strings in the profiles are render fixtures.
2. Provision PostgreSQL independently with encrypted, recoverable customer storage. Create a same-namespace Secret containing a restricted runtime account DSN using `sslmode=verify-full` and the approved database CA. Match the Secret name and keys in `postgres`. Arrange any schema/bootstrap work through the existing database administrator procedure and verify database readiness before customer acceptance.
3. Set the exact DNS host and custom Okta issuer, SPA client ID and API audience. Register `https://<publicHost>/login/callback` and the console origin for the existing production PKCE and IdP MFA flow. Apply server-owned PostgreSQL memberships before sign-in acceptance.
4. For Ingress, supply the existing TLS Secret, ingress class and exact ingress-controller source namespace and pod labels. For external gateway, apply both profiles in order, terminate TLS at the customer gateway, preserve the original Host and route `/api` to the API and `/` to the console. Neither edge needs a route to the operations service. Review customer egress policies for DNS, PostgreSQL and the IdP.
5. Enable monitoring only when the Prometheus Operator CRDs and matching scraper identities are available. Configure operator selectors to discover the chart resources. Review CNI enforcement and verify scrape and alert delivery before treating alerting as operational.

The chart creates no database, Fabric PVCs, ServiceAccount grants or Secret objects. It installs stateless API/console workloads, web traffic policies and optional scraper resources. Customer-selected Fabric classes are assessed independently by [storage planning](../internal/topology/customer_storage.go).

## Reproduce software checks

From the control-plane directory:

```powershell
go test -count=1 ./...
go vet ./...
python -B tools/sprint8/check_helm.py --helm helm
python -B -m unittest discover -s tools/sprint8 -v
go run ./cmd/fabric-maintenance -action verify-certificate -input leaf-and-intermediates.pem -trust-roots root.pem -endpoint peer.example.test -warn-before 720h
python -B tools/sprint8/soak.py --port 9090 --seconds 600 --rate 5 --workers 8
```

The certificate command reads local public PEM and reports a fingerprint, earliest effective chain expiry and renewal status. A trust failure exits nonzero. The soak harness sends only loopback GET readiness probes. Its gates are at least 99% HTTP 204 and at least 95% of planned probes successfully completed within 500 ms, counting dropped work against both. The HTTP service alert thresholds are 1% 5xx and 500 ms p95 handler latency. The 60-second fixture run validates the harness. Extended and production-scale runs remain outstanding.

Run `npm test` and `npm run build` from the console directory. Inspect [telemetry tests](../internal/telemetry/http_test.go), [upgrade guards](../internal/rollout/upgrade_test.go), [mocked image-patch tests](../internal/rollout/patch_test.go) and [certificate tests](../internal/maintenance/certificate_health_test.go) for detailed assertions.

## Release dependencies

Customer Kubernetes acceptance requires the application-controlled customer target adapter, verified CSI and CNI, real ingress and tenant authorization, release-reviewed Fabric image pairs, live certificate/recovery drills and a sustained workload with cross-peer commit verification. Existing real Okta, historical-key remediation, replacement-cluster recovery and multi-node qualification gates retain their prior status.
