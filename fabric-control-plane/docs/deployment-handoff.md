# UI JSON to Bevel handoff

This document describes the disabled **cloud** candidate path. The real local Kubernetes path uses the same canonical JSON but executes through the pinned Bevel Fabric operator; see [local qualification and Windows setup](local-kubernetes.md). Preparing a cloud candidate does not dispatch that local worker.

The console now serializes the full guided cloud design in the versioned `FabricNetwork` JSON. `spec.deployment.cloud` records the account and role identifiers, region, cluster choice, sizing, network ranges, add-ons and DNS domain. It contains no access keys, passwords, tokens or kubeconfig. Server-side target registration remains authoritative for access and placement; UI identifiers cannot override another tenant's registered target.

On the review screen, **Prepare Deployment JSON** validates and saves the current design, then calls `POST /api/v1/networks/{id}/deployment-package` with `If-Match`. The response includes the saved revision, canonical network JSON and its SHA256. When a registered target passes static checks it also includes Bevel input and suspended Kubernetes Job resources. Missing or incompatible targets produce explicit checks, without fabricated Bevel inputs or infrastructure. Preparing a package does not create containers.

The compiler maps organizations to Bevel organizations and CA services; nodes to peer/orderer services; channel references to participants, endorsers and ordering members. Registry configuration supplies Kubernetes context, DNS, Vault bindings and GitOps secret references. Ansible accepts the generated JSON through `-e @bevel-input.json`; conversion to YAML is unnecessary. No browser-provided playbook or executable command is accepted.

For an independently obtained deployment package, the preparation CLI verifies the complete file's expected SHA256, verifies its network digest, resolves the tenant target, and recompiles inputs rather than trusting embedded variables or Kubernetes resources:

```powershell
go run ./cmd/fabric-cloud-prepare -package network-deployment.json -sha256 <expected-package-file-sha256> -registry <trusted-registry.json> -tenant <trusted-tenant> -output <new-private-directory>
```

The expected package-file digest must come from the trusted dispatcher or artifact registry. `networkSHA256` inside the package hashes the canonical network object, not the complete downloaded file; a digest establishes integrity, not authorization. The output directory must not exist. It receives `network.json`, `bevel-input.json`, `kubernetes.json` and `readiness.json`. These files contain internal infrastructure references and must be protected accordingly. The CLI does not implement the execution worker or apply Kubernetes resources.

## Remaining execution work

The next execution layer must persist an authorized operation and immutable artifact reference, acquire a network lease, dispatch a digest-pinned Linux worker to the registered cluster, execute maintained Bevel Ansible roles, and track the resulting Helm/Flux resources. Network readiness must separately verify Fabric CA enrollment, orderer participation, peer channel membership and an endorsed transaction committed and queried on both peers. Kubernetes Job completion alone is not Fabric readiness.

The inspected fork still emits Flux v1 resources and does not admit Fabric 2.5.15. Its source and runtime compatibility need modernization and real deployment testing. The existing candidate Job references a proposed worker executable, not the preparation CLI. Jobs remain suspended, and cloud execution remains disabled. No configured EKS target or cloud credentials are currently available. New cluster provisioning and Azure execution are also unimplemented. The toolchain image verifies tool availability and playbook syntax, not these execution requirements.
