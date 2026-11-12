# Sathonate Fabric SMP — Engineering Handbook

Source baseline `f0305695646d91c765d20df60e6a9f1968cf2eba`. The Go control plane and React console are sibling projects in the Sathonate Git repository. Git is the source of truth. Use this handbook to understand what runs, what it owns and what evidence supports it.

| Section | Pages |
|---|---|
| Architecture | [01 System design](01-system.md) · [02 Codebase map](02-codebase.md) · [03 Workflow sequences](03-workflows.md) · [04 Fabric and Kubernetes](04-fabric.md) |
| Product implementation | [05 API and orchestration](05-api.md) · [06 PostgreSQL](06-postgres.md) · [07 Console](07-console.md) · [08 Security](08-security.md) |
| Operations | [09 Deployment and recovery](09-operations.md) · [10 Commercial operations](10-commercial.md) · [11 Troubleshooting](11-troubleshooting.md) |
| Engineering governance | [12 Evidence and RC1](12-evidence.md) · [13 Decisions and glossary](13-decisions.md) · [Coverage and publication](coverage.md) |

**Evidence vocabulary**

- **Implemented** means code exists in the pinned source. An operator may still need to configure it.
- **Fixture-tested** means controlled inputs, intercepted browser requests, simulated OIDC, or fake infrastructure tested the contract.
- **Locally verified** means actual local components were exercised on the reported revision and topology.
- **Live-verified** applies only to the specific external environment and gate named in its acceptance receipt.
- **Blocked** identifies an unmet prerequisite. **Planned** identifies work without an available execution path.

Sprints 6–9 remain PARTIAL. RC1 production release remains blocked. Consult the [release gate register](../rc1-readiness.md) before changing operating mode.
