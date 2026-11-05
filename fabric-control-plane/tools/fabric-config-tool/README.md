# Dynamic Fabric configuration helper

This isolated Go module generates public Fabric 2.5 / V2_0 Raft channel
genesis blocks and unsigned additive configuration-update envelopes. It is
not a provisioning service and does not qualify Sprint 4 runtime acceptance.
It has no AWS, Kubernetes, network, enrollment, signing or private-key API.

Input is one strict JSON document on stdin, at most 1 MiB. Fields are
`namespace`, `channel`, `organizations` (`MSP`, `Root`, `TLSRoot`, `Anchors`),
`applicationMSPs`, `orderers` (`Name`, `MSP`, `TLS`, `Sign`), and optional
`current`. Certificate fields contain public PEM certificates. `current`
contains a base64-encoded fetched CONFIG **block**, not a raw Config message:
its hash, header type and channel ID must match. The protected caller must
independently establish namespace ownership, desired-intent approval, trusted
issuer provenance and authenticated retrieval of that block.

Output has `genesis` or `update` (base64 protobuf bytes) and `changed`.
An identical configuration produces `changed: false`. Genesis requires an
odd number of at least three orderers. Updates permit one additional Raft
consenter at a time, including an intermediate even-sized set. They reject
member removal, root/MSP replacement, existing-consenter replacement and
changed Raft options. Updates include the ordering endpoints and anchors.
Existing policies and configuration versions are preserved. No signatures
are generated: the existing channel's authorized administrators must review
and sign any proposed update through the separately protected workflow.

The protobuf-v1 configuration dependency is pinned to the controller's fork
and kept in a separate binary to avoid protobuf registry conflicts with the
control plane's protobuf-v2 Fabric client. Neither the original controller
nor the previously qualified candidate is modified.

From this directory:

```powershell
go test -count=1 ./...
go vet ./...
$env:GOOS='linux'; $env:GOARCH='amd64'; $env:CGO_ENABLED='0'
go build -trimpath -o ../../bin/fabric-config-tool .
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED
```

Tests decode the actual generated protobufs and cover three independent
organization/orderer configurations, anchors, additive application membership,
ordering endpoints, single-consenter changes, no-op retries, foreign channel
and namespace rejection, malformed blocks, trust rejection and removals.
They do **not** deploy A/B/C, submit Fabric transactions, qualify running
membership transitions, or establish application/dashboard integration.
