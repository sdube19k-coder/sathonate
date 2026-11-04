Current secure Sprint 1 acceptance: see [Sprint 1 completion](sprint1-completion.md). The report below records the historical 21 October workflow, not the current bearer-token qualification.

# Verification — 21 October 2026

Sathonate Fabric SMP uses separate React and Go repositories on E:. The six-step wizard now follows the supplied PDF step-one shell: white header, light vertical navigation, and one bordered content panel. Desktop geometry and mobile overflow are covered by browser tests. GCP remains visibly unavailable; cloud credentials are not collected in ordinary network JSON.

## Executed checks

- `npm test`: 9 tests passed.
- `npm run build`: passed.
- `npm run format:check`: passed.
- `npm exec playwright -- test e2e/reference-design.spec.ts e2e/pdf-flow.spec.ts -g 'reference shell|PDF cloud|mobile layout' --output E:/Sathonate/.cache/design-reference/browser-checks`: 3 passed.
- With `SMP_LOCAL_E2E=1`, `npm exec playwright -- test e2e/local-kubernetes.spec.ts`: 1 passed in 3.6 minutes against real kind, PostgreSQL, and Fabric workloads. This deliberately interrupts a worker, retries deployment, verifies a committed transaction and matching peer ledgers, checks peer failure/recovery, then removes its own network.
- `go test ./...`, `go vet ./...`, and real PostgreSQL persistence/deployment tests all passed in the final repeat using the repository's E: Go cache.
- Latest API build smoke-tested on loopback port 8081, including rejecting inline credentials and invalid readiness request bodies. The temporary smoke server was stopped afterward.

## Real Fabric evidence

Operation: `op_450b58704d35f38bd12c005aa90ea9fb`.
Transaction: `d8874471c9483f5198e329ae149f7491bd99ae809c838030e1813b10ea040d4a`.
Both peer ledgers reached height 9. The network included two TLS certificate authorities, two peers, three Raft orderers, a channel, and sample chaincode. The test deployment is now `removed`.

The retained network, `Local acceptance 1792206274622`, remains healthy with two peers, three orderers, and two CAs. Its operation is `op_9ac07bd1f386c3a692efb5a02082bca8`. UI port 5173 returned HTTP 200; API port 8080 returned ready=true for that network after the test.

## Visual evidence

- `E:/Sathonate/.cache/design-reference/step-1-comparison.png`: PDF reference and implemented AWS step one.
- `E:/Sathonate/.cache/design-reference/all-six-steps.png`: actual six-step local deployment screenshots.
- `E:/Sathonate/.cache/design-reference/final-local-step-1.png` through `final-local-step-6.png`: individual screenshots.

## Explicit limitations

AWS account/profile/EKS/context and Okta application/domains do not yet exist. No paid resources were created. Cloud readiness checks are read-only and report configuration separately from live verification. Cloud execution is disabled: the cloud adapter and production qualification are still outstanding, so identifiers alone do not make this a production cloud deployment product. See the backend `docs/cloud-launch.md` for prerequisites and commands.

Automatic approval review blocked restarting the existing API process without a specific reason. That API was preserved; the latest binary was tested separately on 8081. The running 8080 build supports local deployment and readiness, but predates the latest strict readiness POST body validation. A normal restart is needed to load that final handler change.
