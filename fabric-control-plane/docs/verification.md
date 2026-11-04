# Verification and limitations

Verified locally on 15 October 2026 with Go 1.26.7 on Windows.

The dedicated local Docker runtime is now verified with real Fabric CA enrollment (no cryptogen), three Raft orderers, two peers, a joined channel and a Go shim contract. Endorsed writes committed and were queried on both peers. Stop/start retained the peer MSP certificate and ledgers; both peers reported identical block height/hash afterwards. Ten containers remain running. This fixed local profile is separate from arbitrary saved designs. See local-runtime.md.

The Go tests cover: topology validation; unsafe quorum/failure-domain rejection; deterministic plan identity; no Bevel dependency in native plans; persisted draft/review reopen; stale edits/archives; concurrent editors with one winner; duplicate network identifiers; clone isolation; failed writes leaving memory unchanged; corrupt-store startup failure; and API body/Host/Origin/revision boundaries.

The separate React repository has browser tests against this real API. Draft tests create/save, review, export, edit, reload and archive without provisioning. Local-runtime integration tests submit actual contract writes and stop/reconcile the existing Docker network through review/create/progress/dashboard.

Additional unit checks cover RS256 access-token verification, issuer/audience/expiry/client/tenant rejection, role-based write authorization, tenant-scoped target catalogs, Bevel candidate compilation, suspended job security, secret-reference handling and read-only preflight command boundaries. HTTP checks verify missing/stale cloud revisions, unregistered-target refusal and that checks do not mutate drafts.

Optional Okta and PostgreSQL paths are implemented, but no company Okta application or running PostgreSQL instance was available for qualification. PostgreSQL integration tests are gated by SATHONATE_TEST_DATABASE_URL and were skipped locally. Cloud command checks use mocks; no EKS resources or cloud Fabric networks were created. The dedicated local Fabric network is real.

Local operation journals, staged execution, request deduplication, linked retries and live Fabric/CA observations are now implemented. Tests cover durable lifecycle/reopen, interrupted recovery, retry linkage, tenant isolation, one concurrent winner, failed persistence and credential redaction. The executor remains a single local API process.

Not implemented: executable cloud/multi-host workers, signed release bundles, distributed operation scheduling, production certificate lifecycle management or production Kubernetes observations. Backup/restore, power-failure recovery, multi-instance storage behavior and infrastructure tenant isolation remain unqualified. The default file-based local mode must stay on loopback. See integrations.md and spec-alignment.md for remaining gates.
