# API route catalog

[Handbook](README.md) · [API contracts](05-api.md)

Route registration at `f0305695646d91c765d20df60e6a9f1968cf2eba`. Wrappers may add authentication, entitlements and runner gates. The operations listener is defined separately.

| Route | Implementation |
|---|---|
| `GET /api/v1/customer` | [a.customerAccount](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L40) |
| `POST /api/v1/networks/{id}/customer-preflight` | [a.customerProvisionPreview](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L41) |
| `POST /api/v1/customer/actions` | [a.customerAction](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L42) |
| `POST /api/v1/customer/invitations/accept` | [customerAction](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L43) |
| `GET /api/v1/audit` | [func](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L44) |
| `GET /api/v1/local-runtime` | [a.localStatus](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L62) |
| `GET /api/v1/local-runtime/health` | [a.localHealth](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L63) |
| `GET /api/v1/operations` | [a.localOperations](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L64) |
| `GET /api/v1/operations/{operationId}` | [a.localOperation](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L65) |
| `GET /api/v1/operations/{operationId}/events` | [a.localOperationEvents](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L66) |
| `GET /api/v1/operations/{operationId}/diagnostics` | [a.operationDiagnostics](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L67) |
| `POST /api/v1/operations/{operationId}/retry` | [a.localRetry](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L68) |
| `POST /api/v1/local-runtime/start` | [a.localStart](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L69) |
| `POST /api/v1/local-runtime/stop` | [a.localStop](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L70) |
| `POST /api/v1/local-runtime/test` | [a.localTest](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L71) |
| `GET /api/v1/local-runtime/logs/{node}` | [a.localLogs](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L72) |
| `GET /api/v1/projects` | [func](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L73) |
| `GET /api/v1/auth/identity` | [func](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L81) |
| `GET /api/v1/auth/config` | [func](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L88) |
| `GET /api/v1/cloud/targets` | [a.cloudTargets](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L89) |
| `GET /api/v1/readiness` | [a.launchReadiness](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L90) |
| `POST /api/v1/readiness/probe` | [a.launchReadiness](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L91) |
| `POST /api/v1/networks/{id}/cloud-preflight` | [a.cloudPreflight](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L92) |
| `POST /api/v1/networks/{id}/cloud-bundle` | [a.cloudBundle](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L93) |
| `POST /api/v1/networks/{id}/deployment-package` | [a.deploymentPackage](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L94) |
| `POST /api/v1/networks/{id}/deploy` | [func](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L95) |
| `POST /api/v1/deployments/{operationId}/cancel` | [a.deploymentCancel](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L100) |
| `GET /api/v1/deployments/{operationId}/lifecycle/{action}/preview` | [a.lifecycle](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L101) |
| `POST /api/v1/deployments/{operationId}/lifecycle/{action}/deploy` | [func](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L102) |
| `GET /api/v1/networks/{id}/dynamic/preview` | [a.dynamicNetwork](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L107) |
| `GET /api/v1/deployments/{operationId}/workloads` | [a.deploymentWorkloads](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L108) |
| `GET /api/v1/deployments/{operationId}/recovery/preview` | [a.nodeRecovery](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L109) |
| `POST /api/v1/deployments/{operationId}/recovery/deploy` | [a.nodeRecovery](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L110) |
| `POST /api/v1/networks/{id}/dynamic/deploy` | [func](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L111) |
| `GET /api/v1/deployments/{operationId}` | [a.deploymentStatus](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L116) |
| `GET /api/v1/deployments/{operationId}/health` | [a.deploymentHealth](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L117) |
| `GET /api/v1/deployments/{operationId}/logs/{node}` | [a.deploymentLogs](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L118) |
| `POST /api/v1/deployments/{operationId}/rotation-plans` | [a.rotationPlan](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L119) |
| `GET /api/v1/deployments/{operationId}/rotation-plans/{planId}` | [a.rotationPlan](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L120) |
| `GET /api/v1/deployments/{operationId}/cleanup-preview` | [a.deploymentCleanupPreview](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L121) |
| `DELETE /api/v1/deployments/{operationId}` | [a.deploymentCleanup](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L122) |
| `GET /api/v1/meta` | [func](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L123) |
| `GET /api/v1/networks` | [func](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L138) |
| `POST /api/v1/networks` | [a.create](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L146) |
| `POST /api/v1/validate` | [a.validate](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L147) |
| `POST /api/v1/plans` | [a.plan](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L148) |
| `GET /api/v1/networks/{id}` | [a.get](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L149) |
| `PUT /api/v1/networks/{id}` | [a.update](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L150) |
| `DELETE /api/v1/networks/{id}` | [a.archive](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L151) |
| `POST /api/v1/networks/{id}/review` | [a.review](https://github.com/sdube19k-coder/sathonate/blob/f0305695646d91c765d20df60e6a9f1968cf2eba/fabric-control-plane/internal/httpapi/api.go#L152) |

[Separate operations listener](../../internal/telemetry/http.go)
