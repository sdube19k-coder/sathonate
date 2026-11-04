# HTTP contract — local development v0.1

All routes start with `/api/v1`. The API returns JSON and `Cache-Control: no-store`. No CORS wildcard is enabled. The console uses its own same-origin development proxy.

GET /meta explicitly returns `executionEnabled: false`, the configured authentication mode/storage and the schema namespace. UI capabilities must follow that boundary rather than showing simulated provisioning.

POST /networks and PUT /networks/{id} accept the desired `FabricNetwork` specification directly. Required top-level keys: apiVersion, kind, metadata, spec. Unknown keys, including inline credentials, are rejected. Payloads are limited to 1 MiB. Validation errors return field-level issues with severity/path/message.

Successful draft response:

```json
{
  "id": "server-generated-record-id",
  "revision": 1,
  "status": "draft",
  "desired": { "apiVersion": "fabric.sathonate.io/v1alpha1", "kind": "FabricNetwork", "metadata": {}, "spec": {} },
  "issues": [],
  "events": [],
  "createdAt": "RFC3339 timestamp",
  "updatedAt": "RFC3339 timestamp",
  "archived": false
}
```

The empty metadata/spec above illustrate the response envelope only, not a valid desired specification. Valid complete inputs are in examples/.

A review adds `plan` and changes status to `reviewed`, never `deployed`. It does not advance the desired revision. Repeating an identical review produces no duplicate activity event. Updating the design increments the revision, clears the review, and appends an activity record. Archived drafts leave active lists; the stored record remains retrievable by ID.

PUT, DELETE and POST /networks/{id}/review require If-Match containing the desired revision. A revision conflict requires reloading before applying changes; never auto-overwrite. DELETE archives the record and responds 204. Creating a duplicate active desired network identifier returns 409.

Error envelope:

```json
{ "error": { "code": "validation", "message": "Some network settings need attention", "issues": [] } }
```

Codes include invalid_json, validation, duplicate_identifier, immutable_identifier, revision_required, revision_conflict, not_found, invalid_host, invalid_origin, content_type and storage. The server does not return raw secret-bearing payloads or filesystem diagnostics to browsers.

GET /auth/config is public and returns local-development or the non-secret Okta issuer/client/audience configuration. Other routes require a verified bearer access token in configured Okta mode. Authentication and authorization failures currently use plain HTTP 401/403 errors; clients must tolerate non-JSON error responses. Default loopback development supplies one local administrator identity.

GET /cloud/targets returns only the authenticated tenant's registered targets. POST /networks/{id}/cloud-preflight and /cloud-bundle require current If-Match. Preflight is read-only, returns `ready: false`, and reports missing/unsupported infrastructure explicitly. Bundle export requires an authorized operator and a registered compatible target; the result has `executable: false` and contains only candidate configuration and suspended staging resources. Neither endpoint applies resources or changes draft state. See integrations.md for remaining adapter limitations.

The native runtime is initially linux-containers. Bevel planning requires kubernetes with aws or azure provider. Both are planning contracts only. Placement and CA references are placeholders until authenticated inventory resolution exists.

## Ordering configuration

`spec.ordering` contains `consensus` (`raft` or `bft`) and `channelCapability` (`V2_0` or `V3_0`). Newly saved drafts persist both. Older drafts without ordering retain their implicit Raft interpretation; stale pre-ordering review plans are invalidated on loading.

Bevel planning is limited to Fabric 2.5.x/Raft. Native planning accepts exact 2.5.x or 3.x versions for Raft; SmartBFT requires native Fabric 3.x and V3_0. None of these profiles is execution-certified. Solo is not accepted for new designs: it was test-only and removed in Fabric 3.0.

Orderer nodes use the existing node role/organizationRef/placementRef/failureDomain fields; per-channel ordererRefs identify consenters. Production Raft policy requires odd counts of at least three. BFT profiles use 3f+1 counts with f >= 1; per-domain loss must stay within the corresponding fault budget. The server also warns if one organization controls more BFT orderers than the fault budget. Real placement/organization independence remains an execution-time verification responsibility.

Sources: [Fabric 3.0 release notes](https://github.com/hyperledger/fabric/blob/main/release_notes/v3.0.0.md), [BFT configuration](https://hyperledger-fabric.readthedocs.io/en/latest/bft_configuration.html), [ordering service](https://hyperledger-fabric.readthedocs.io/en/latest/orderer/ordering_service.html).

## Operation diagnostics (Sprint 4)

`GET /api/v1/operations/{operationId}/diagnostics` requires an authenticated
server-owned project grant and read permission. It returns 404 for an absent or
foreign operation and uses `Cache-Control: no-store`. PostgreSQL history-only
reads and immediate membership revocation use the same guards as operation
history. The implicit fixed developer identity cannot download these reports.

The `sathonate.operation-diagnostics/v1` JSON contains recorded operation status,
IDs, timestamps, stage status/timing, retry/cancellation metadata and fixed
next-step guidance. `evidenceType` is `recorded-operation` and
`liveHealthChecked` is always false. Event/error text, actors, credentials,
designs, Kubernetes resources and logs are excluded. Refresh deployment health
separately to observe current workloads. Downloading a report starts no job and
does not retry, cancel, modify or delete infrastructure.
