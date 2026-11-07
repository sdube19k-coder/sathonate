# Sprint 5 completion checkpoint — 8 November 2026

**PASS: bounded local operations acceptance. No active fixture remains. Do not rerun completed tests.** See [report](sprint5-acceptance.md) and [final evidence](evidence/sprint5-final/summary.json).

- Node recovery evidence is preserved at `dd0a913`: peer, Raft orderer and CA each passed, with fresh transactions, equal ledgers and original PVC/unrelated-node preservation.
- Minimum cold run `dynamic-browser-s5-20261107-191711` passed seven original-PVC restorations, two missing captured peer identity Secrets and scoped control rows. Prior tx `e92089065e182f39176346542f4672991defe0c80013ffa8fcbb92db8de43928` remained VALID on both peers. Fresh tx `04374547c9645c12d6ee497c767babab85b78dd3b58ae16c892f428a91fc24e7` reached equal height 6/hashes. Its subsequent deletion transport failure remains explicitly separate.
- Final deletion-only run `dynamic-browser-s5-20261107-195129` passed. It repeated neither cold recovery nor node fault tests. Browser approval removed 17 exact workload/execution resources, retained 126 recorded objects, preserved all Secret bytes and original Bound PVCs, rejected unauthorized/stale/foreign access and acknowledged completion idempotently. Actual TLS, Calico isolation, RBAC and credential inspections passed.
- Earlier report-writer, expired negative-test grant and deletion-only API-helper harness failures are documented. The backend HTTP write deadline mismatch was fixed only for bounded authenticated local cleanup and regression-tested on a real HTTP server.
- Final Go/vet, serial PostgreSQL, console/build/format and Python tests/static checks passed. See exact counts/skips in the final evidence.
- Cleanup recheck on 8 November found only retained `smp-qualification`, eight ready workloads and seven Bound PVCs. No retained mutation occurred. Fixture processes, encrypted private artifacts and runtime file are gone.

The original controller and Sprint 4 worker are preserved. The separate Sprint 5 diagnostic worker's 39 compiled local source files match its receipt. No AWS, paid or production operations occurred.

Cold recovery remains original-namespace/PVC only, restricted to loopback `fabric_smp_tests`; no complete PostgreSQL or replacement-cluster disaster recovery. Retained deletion is not permanent erasure or qualified reactivation. Real Okta/MFA, historical-key remediation, EKS/multi-server storage and production gates remain open. Sprint 3 stays open. No further sprint is started by this checkpoint.

Implementation commit: `f0936f6510c19740f3460a6217f02f10a1fe1894`. Evidence is recorded separately so runtime receipts bind to an immutable source revision.
