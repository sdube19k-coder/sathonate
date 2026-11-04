# Control plane rules

- Go backend only; frontend lives in E:/Sathonate/fabric-console and communicates over HTTP.
- Keep desired topology, observations and operations separate. Never report draft saving/planning as deployment.
- Never accept inline credentials or user-controlled executable templates. Do not log request payloads.
- Current file store and loopback server are local-development facilities, not production persistence/authentication.
- Preserve revision checks on draft updates. Test stale revisions, persistence failure and isolation boundaries.
- Cloud execution uses Bevel; native execution is independent. A dedicated CA-enrolled local Docker development profile now runs real nodes; arbitrary saved cloud/native designs still have no execution adapter.
- Run go test ./... and go vet ./... after Go changes.
