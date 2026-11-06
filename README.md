# Sathonate

Private source repository for the Sathonate Fabric control plane and React console.

## Components

- `fabric-control-plane/`: Go API, protected application-controlled Fabric workers,
  persistence, authorization, qualification scripts and redacted engineering evidence.
- `fabric-console/`: React/TypeScript console using the versioned HTTP API.
- `dependencies/bevel-operator-fabric`: pinned upstream submodule; clone with
  `git submodule update --init --recursive` when following the build instructions.

## Qualification status

**Sprint 4 dynamic local Raft MVP PASS (5-6 November 2026).** One customer-configuration engine now provisions and reconciles organizations, peers, authorities, Raft orderers, channels, anchors and approved chaincode artifacts. Three distinct customer topologies, independent running expansions, actual executable upgrades, authenticated API/Edge dashboard acceptance and runtime security/cleanup passed.

See [current acceptance](fabric-control-plane/docs/sprint4-dynamic-acceptance.md), [source-linked evidence](fabric-control-plane/docs/evidence/sprint4-dynamic-final/summary.json) and the [configuration/API guide](fabric-control-plane/docs/sprint4-dynamic-local.md). The fixed topology remains a regression fixture.

Production release remains **BLOCKED** by real Okta acceptance, historical-key remediation and infrastructure/storage qualification. Sprint 3 remains **LOCAL QUALIFICATION PASSED / AWS EKS QUALIFICATION PENDING**. BFT, removals and replacement operations are unqualified; no unsupported consensus is advertised. Sprint 5 begins with the authorized local operations/recovery backlog. AWS and production remain disabled.

## Local development

Follow the component README and engineering instructions. Existing qualification
scripts currently use the Windows `E:/Sathonate` development layout; this import
preserves the verified source and does not claim portable path qualification.
The console development origin is `http://127.0.0.1:5173`; API integration uses
the loopback Go API at `http://127.0.0.1:8080`.

```powershell
cd fabric-control-plane
go test ./... -count=1
go vet ./...
cd ../fabric-console
npm ci
npm test
npm run build
```

Authentication must use explicitly configured local bearer grants or configured
Okta with server-owned project membership. Never commit tokens, private keys,
passwords, kubeconfigs, database contents or private runtime evidence.

## Import provenance

This is a clean source snapshot, not a mirror of earlier project Git history.
The source commit IDs and archive hashes are in `SOURCE_PROVENANCE.json`.
Original local repositories, existing SSH keys and the prior GitHub account are
preserved. Uncommitted/unverified AWS preparation drafts are excluded.
