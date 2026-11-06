# Customer-defined local Fabric networks

The explicitly enabled `smp-dynamic-local-v1` profile consumes customer specifications on a trusted disposable local target. Saving a design is not deployment. The original fixed topology remains a regression fixture. AWS and production are disabled for this profile.

## Configure and deploy

Use the authenticated API or **Networks > Customer configurations** in the console. Select a server-authorized tenant/project. Existing local bearer authentication requires explicit local-development configuration and server-owned hashed grants. Live Okta MFA acceptance remains separately blocked.

`GET /api/v1/meta` exposes `dynamicLocal`: approved public placements, volume limits and immutable code artifacts. It never supplies signing keys, credentials or kubeconfig. Resolve the placeholder placement/artifact values in `examples/dynamic/a.json`, `b.json` or `c.json` against that trusted inventory before deployment.

Each design specifies organizations/MSPs, peers/orderers, per-node resources/storage, combined or split signing/TLS authorities, channels/members, anchor peers and chaincode/endorsement thresholds. The enabled profile uses Fabric 2.5.15 Raft/V2_0. Supported channels require an odd number of at least three orderers, a managed anchor for every member organization and at least one approved contract.

The current envelope is at most four organizations, twelve peer/orderer nodes, four channels, eight authorities and eight chaincodes, subject to lower aggregate quotas and volume limits. Placements select existing inventory; they do not create servers. Public designs must not contain passwords, tokens, private keys or executable templates.

## API sequence

1. `POST /api/v1/networks` saves desired configuration. Keep its server record ID and revision. Later `PUT /api/v1/networks/{id}` requires `If-Match`.
2. `POST /api/v1/networks/{id}/deploy` with the saved revision in `If-Match` and an actor-bound `Idempotency-Key` starts initial execution. Read `operationId` from the response.
3. Read `/api/v1/operations/{operationId}` for durable stages/events and `/api/v1/deployments/{operationId}` for completed observations. HTTP 202 does not prove success.
4. Save additive desired changes. Request `GET /api/v1/networks/{id}/dynamic/preview?parent={lastVerifiedOperationId}` with current `If-Match`. Review full before/after intent and recovery semantics.
5. Apply `POST /api/v1/networks/{id}/dynamic/deploy` with the same revision, a fresh idempotency key and body `{"parentOperationId":"...","approval":"<preview digest>"}`. Changing the actor, parent, target, revision or design invalidates approval.
6. After interruption, use the existing recorded-operation retry action. Do not create a replacement network. API restart adopts the owned Job. Each contract/channel result contains its transaction ID, executable/artifact/package identity, ledger heights/hashes and orderer status.

Read grants do not authorize deployment/retry. All requests, including status and retries, retain server-owned tenant/project checks. Cross-project/tenant records are hidden.

## Safe changes and qualification limits

Additive reconciliation supports peers, organizations/authorities, channel membership, anchors, channels, Raft members and sequential actual code upgrades. Raft members enter one at a time and activate/catch up before the next admission. Existing identities, PVCs and previous code services remain available. Already committed Fabric updates recover forward; rollback is not promised.

Executable images must already be server-approved with an immutable digest and binary SHA256. Customers cannot submit arbitrary runnable images. Code upgrades use a different approved binary/artifact and actual package; definition metadata alone is insufficient proof.

Removals, root replacement, storage/placement replacement, private collections and BFT execution are unqualified. Dynamic API deletion remains unavailable; trusted disposable teardown checks cluster identity. Genuine multi-server and EKS/IRSA/CNI/EBS acceptance remain pending. Dashboard ledger observations are explicitly recorded evidence, not live metrics.

See `docs/evidence/sprint4-dynamic-final/summary.json` and `docs/sprint4-lifecycle-checkpoint.md` for executed acceptance, failures and resume instructions. Preserve the retained network and its data.
