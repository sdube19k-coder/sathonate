# 11 · Troubleshooting

[Handbook](README.md) · [Operating limits](09-operations.md) · [RC1](../rc1-readiness.md)

| Symptom | Diagnostic and safe resolution | Escalation |
|---|---|---|
| Okta callback or MFA fails | Check the exact configured issuer, redirect URI, assigned app and approved assurance policy in the IdP. Correlate redacted outcomes in the IdP log. Verify HTTPS and the configured callback. | Okta administrator and account owner |
| Authenticated user cannot select project | Inspect server-owned membership for verified issuer/subject/environment and active anchor. Check token age after a newly granted membership. Never substitute group or email authorization. | Identity administrator |
| Request returns 409 or 428 | Reload current draft revision, inspect existing operation and exact preview digest, then retry an ambiguous request with the original idempotency key. | Control-plane engineer |
| Provisioning stopped or stale | Read scoped operation events and diagnostics, compare durable target binding and current namespace identity. Resume only the approved operation. | Fabric operator |
| Endorsement, quorum or ledger disagreement | Compare current channel members, package/definition, peer heights, endorsed MSPs and orderer voting state. Check DNS/TLS using approved public roots. Stop further mutation if ledger or quorum diverges. | Fabric org/channel owners |
| PVC pending or node failure | Inspect scheduling events, storage class, PVC UID and local-volume node affinity on the explicit approved context. Do not move or recreate a bound volume by assumption. | Customer Kubernetes owner |
| Backup lock or pending restore | Verify the prior process stopped, reconcile the immutable checkpoint and artifact digest, and confirm writer fence and ownership before resuming. | Recovery operator and security |
| Duplicate payment, unavailable entitlement or metering | Read the deduplicated audit outcome and job aggregate. Use the same verified event or scope/window for safe retry. Investigate provider mapping and DB grants. | Billing engineer and DBA |

Illustrative read-only Kubernetes inspection on an approved disposable context:

```text
kubectl --kubeconfig <approved-file> --context <approved-context> -n <approved-namespace> get pods,pvc,deployments
kubectl --kubeconfig <approved-file> --context <approved-context> -n <approved-namespace> get events --sort-by=.metadata.creationTimestamp
```

Never include kubeconfig contents, tokens, certificate keys or raw controller logs in evidence. [Diagnostics handler](../../internal/httpapi/api.go) · [Operations and recovery runbooks](09-operations.md).
