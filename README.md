# Sathonate

Private source repository for the Sathonate Fabric control plane and React console.

## Components

- `fabric-control-plane/`: Go API, protected application-controlled Fabric workers,
  persistence, authorization, qualification scripts and redacted engineering evidence.
- `fabric-console/`: React/TypeScript console using the versioned HTTP API.
- `dependencies/bevel-operator-fabric`: pinned upstream submodule; clone with
  `git submodule update --init --recursive` when following the build instructions.

## Qualification status

The bounded local lifecycle increment is verified: approved third-peer addition,
chaincode definition sequence upgrade, UID-checked peer restart, new committed
transactions, API restart/adoption, tenant authorization and Kubernetes isolation.
See [acceptance and limitations](fabric-control-plane/docs/sprint4-lifecycle-acceptance.md)
and its [source-linked evidence](fabric-control-plane/docs/evidence/sprint4-lifecycle-verification.json).

**Sprint 4 remains PARTIAL.** Generic customer-configured provisioning,
organization/Raft membership changes, general channel/anchor management, actual
chaincode code upgrades and BFT execution remain unfinished. Real Okta acceptance
and historical key remediation remain open production gates. Sprint 3 AWS EKS
qualification is deferred. Production and cloud deployment must remain disabled.

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
