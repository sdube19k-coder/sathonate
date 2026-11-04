# Fabric SMP experience

The current UI follows the PDF-based six-step workflow. See pdf-design-system.md for layout decisions, source files and capability boundaries.

Every screen uses the same Sathonate navigation, breadcrumb/account header, six-step indicator, blue-accented white cards, right summary rail and footer controls. The layout reflows into one column on phones, with an accessible navigation drawer and horizontally scrollable step indicator.

1. Cloud Account: conditional provider identity references and actual local engine connection checks.
2. Kubernetes Configuration: new/existing cluster planning or the actual local Docker runtime.
3. Network Design: basics, organizations, peers, Raft orderers, Fabric CA, channels and initial chaincode.
4. Review & Cost Estimate: editable grouped summaries, validation, resource counts and topology; no fabricated price quote.
5. Deployment Progress: actual persisted stage state, details, events, failure/retry and verification.
6. Network Dashboard: real health, topology, ledgers, consenters, CAs, contract, verified transaction and logs.

Desired Fabric designs save through the Go API with revision checks. Non-secret account/cluster-planning metadata is retained only in the current browser tab. Cloud authorization, provisioning and pricing are not enabled. Cloud designs cannot progress into a fabricated deployment or dashboard.

Local execution is a fixed profile with Fabric CA identities and real containers. Dashboard access follows completed verification; runtime and operation state restore after refresh. Stop retains identities and ledgers. Custom native hosting and SmartBFT execution remain outside this supported guided flow. Existing unsupported private-server designs are preserved, not silently converted.

No ordinary form collects private keys, permanent cloud credentials or kubeconfig tokens. Unavailable providers/actions are disabled or clearly marked. Error states describe actual API failures; live data is never replaced with sample health or metrics.
