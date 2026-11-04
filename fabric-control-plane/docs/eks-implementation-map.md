# EKS implementation map

- Validation: `internal/domain`; saved revision and tenant authorization: `internal/httpapi/deployment.go` and identity middleware.
- Journal: `internal/operations`, `internal/deployment/manager.go`, PostgreSQL deployments table. The existing local executor uses a database advisory lock; cloud scheduling will retain this single-owner model and resume recorded operations after restart.
- Execution: `internal/deployment/kubernetes.go` launches a Job and consumes JSON events. `internal/kubernetesfabric/worker.go` is the shared Fabric lifecycle, using kubectl-hlf v1.14.0 and the Bevel operator's Helm reconciliation.
- Exact operator source: `cd236ccc61d1d159b3dadb4625d08bb5682a8451`. This is separate from the legacy Ansible/Flux exporter in `internal/cloud/compiler.go`; new operator targets must not use that exporter.
- EKS adaptation: trusted target registry, explicit AWS identity and endpoint checks, approved namespace, encrypted storage, pinned worker/runtime configuration, scoped worker RBAC and a control-plane channel-resource broker (channel CRs are cluster scoped in this operator).
- Gates: qualification-only EKS mode requires a server-owned approval bound to the exact target configuration and successful live preflight. Production cloud execution stays disabled pending real EKS and enterprise qualification. No infrastructure provisioning is part of this adapter.
