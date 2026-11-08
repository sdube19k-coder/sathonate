# UI-to-Fabric acceptance — 8 November 2026

**BLOCKED by local host capacity. This is not an end-to-end PASS.**

[Redacted receipt](evidence/ui-e2e-20261108/summary.json) binds the attempt and regression logs to source commit `97e0cb5`. Existing Sprint 4/5 receipts are preserved and are not counted as current runtime results.

## Actual execution

The current-source Go API was built. The existing browser qualification runner created only disposable cluster `smp-sprint1-f89b1fec`. Windows available RAM fell from approximately 4.19 GiB to **364 MiB** during cluster initialization. Docker's usable total remains approximately 5.78 GiB. The test was stopped before Fabric provisioning, API startup or browser acceptance. No transaction was executed.

The runner was stopped and the exact disposable cluster was deleted. Its generated kind configuration and kubeconfig were removed; no fixture token grants were created. Only retained cluster `smp-qualification` remains. Fresh read-only checks found **8/8 deployments ready and 7/7 PVCs Bound**. No retained resource was changed, no cloud API was accessed and no system memory setting was changed.

| Current check | Result |
| --- | --- |
| Go regressions | 379 PASS, 23 opt-in skips |
| PostgreSQL-backed regressions | 45 PASS, 1 opt-in skip |
| Go vet | PASS |
| Frontend tests / production build | 26 PASS / build PASS; existing chunk warning |
| UI network provisioning and committed transactions | NOT TESTED — capacity stop |
| UI organization/peer/CA/Raft expansion, channels and code upgrade | NOT TESTED |
| UI monitoring, safe node recovery and ledger continuity | NOT TESTED |
| Browser-approved retained-data deletion | NOT TESTED |

The previously captured UI design screenshots prove rendering in read-only history mode, not provisioning or live Fabric acceptance. Cold backup/restore is an offline administrator facility, not an implemented customer UI workflow; it is outside this UI acceptance run. Real Okta and production/cloud gates remain open.

## Resume after freeing host memory

Close memory-heavy applications without restarting WSL or the retained cluster. Recheck Windows/Docker headroom before retrying. The four-node Sprint 6 memory gate remains unchanged; this runner uses a separate single-node disposable cluster.

The existing runner requires ports 8080, 5173 and 5174 to be unused. Stop only the known local screenshot/preview sessions if necessary. Do not stop unrelated listeners. Choose a new evidence label so this blocked receipt is preserved.

Run in normal PowerShell, after capacity has been checked:

```powershell
Set-Location E:\Sathonate\sathonate\fabric-control-plane
$env:GOCACHE='E:/Sathonate/.cache/go-build'
$env:GOPATH='E:/Sathonate/.cache/go'
$env:GOPROXY='off'
go build -o bin/fabric-api-sprint5.exe ./cmd/fabric-api
if ($LASTEXITCODE -ne 0) { throw 'API build failed' }

Set-Location E:\Sathonate\sathonate\fabric-console
$env:SMP_BACKEND_ROOT='E:/Sathonate/sathonate/fabric-control-plane'
$env:SMP_CACHE_ROOT='E:/Sathonate/.cache'
$env:SMP_KIND_PATH='E:/Sathonate/tools/kind.exe'
$env:SMP_UI_EVIDENCE_LABEL='dynamic-browser-ui-full-resume-20261108'
$env:SMP_UI_DYNAMIC='1'
$env:SMP_UI_SPRINT5='1'
$env:SMP_UI_SPRINT5_WORKER='1'
$env:SMP_UI_SPRINT5_OPERATIONS='1'
$env:SMP_UI_SPRINT5_DELETION_ONLY='1'
$env:SMP_UI_CALICO='1'
Remove-Item Env:SMP_UI_SPRINT5_MINIMAL -ErrorAction SilentlyContinue
Remove-Item Env:SMP_UI_SPRINT5_OPERATIONS_ONLY -ErrorAction SilentlyContinue
$env:SMP_UI_API_SHA256=(Get-FileHash ../fabric-control-plane/bin/fabric-api-sprint5.exe -Algorithm SHA256).Hash.ToLower()
python tools/qualify-ui.py
```

This selects initial customer-defined provisioning, reviewed organization/peer/CA/Raft expansion, additional channel and anchor configuration, actual chaincode code upgrade, peer/orderer/CA recovery, TLS/RBAC/Calico and credential inspections, followed by browser-approved retained-data deletion. Negative HTTP probes and destructive fault injection supplement the actual browser actions; they must not be described as all manually performed through the UI. The bounded cold backup/restore test is deliberately not repeated. The harness removes its disposable cluster after normal completion/failure. Independently verify cleanup if the process is interrupted.
