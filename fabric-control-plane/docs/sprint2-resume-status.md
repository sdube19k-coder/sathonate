# Sprint 2 qualification checkpoint — 2026-10-25

Overall: **BLOCKED for closure**. Docker connectivity was restored (29.7.2). Final-source local qualification now passes; real Okta and historical incident gates remain open. No production or AWS deployment is authorized. The retained network and original controller were not modified. The final harness observed retained readiness read-only at **8/8**.

The console changes are committed as `6a50eaf2454a3ad643dfd2dcd03a0d93bb74e456` after unit tests, build, format and source review. This records the implementation; real Okta callback/API/logout acceptance remains blocked rather than implied by that commit. The consolidated evidence identifies the backend commit and canonical source-file hashes.

Fresh fixture-adapted legacy regressions `TestEKSWorkerLifecycleOnKind` and `TestLocalWorkerRegressionOnKind` both PASS in `smp-regression-e20605f7`; its cluster was removed. The first CSV contained only 32 Admin Console events. The second CSV has 319 rows and ten matching SPA failures, all with the duplicated approved username, actor `unknown` and `VERIFICATION_ERROR`. The agent caused the duplicate by appending to a prefilled field. Replacement and visual verification corrected the username. After the user completed an offered security method, Okta required passkey enrollment; the user deferred this step. No authentication policy was changed. No password reset or policy weakening is justified by this finding. A UI-recognized username or log actor is not a verified token subject. The committed redacted record is [evidence/sprint2-local-qualification.json](evidence/sprint2-local-qualification.json); it preserves the earlier atomic evidence and marks real Okta/incident closure blocked.

| Gate | Result and limits |
| --- | --- |
| Real Okta | BLOCKED, explicitly deferred by user. The duplicated username causing the ten earlier SPA verification failures is corrected. The recognized user completed verification and reached **Required now: Passkey** enrollment. No policy change, passkey removal, callback/token acceptance, real-user membership, authenticated API or logout PASS is claimed. |
| Server-owned authorization | PASS: fresh controlled-OIDC/PostgreSQL regressions cover server-owned membership, owned reads, denied writes, foreign scope denial and immediate database revocation. The operation-history guard correction is verified. Real Okta acceptance is separate. |
| Kubernetes isolation | PASS on final source: disposable Calico ingress/egress negative probes with positive controls, RBAC/foreign-resource protections, PSA/resource admission and absent Fabric API token mounts. |
| Certificate trust | PASS on final source: trust/name/role/key authorization regressions and actual live peer certificate checks; does not qualify every Fabric identity class. |
| Atomic recovery | Earlier unchanged candidate PASS: all six failures, 9 stages / 42 checks. Independent final acknowledgement observer PASS: live certificate, recovery/acknowledgement, no redundant Helm upgrade or peer replacement/restart. |
| Historical key exposure | BLOCKED: authorized exposure inventory and coordinated remediation/restore evidence remain outstanding. No retained key rotation performed. |
| Current regression checks | Fresh Go suite (216 tests/subtests; 14 explicit opt-in skips), vet, module verification, candidate 18 checks, console 16 tests/build/format, Python suites and 12 startup denials PASS. Explicit PostgreSQL, verified-TLS/restricted-role Docker and final-source API-to-Fabric suites also PASS. Skips are not runtime PASS. |

The preceding blocked continuation had this access error (resolved on resume):

```text
failed to connect to the docker API at npipe:////./pipe/dockerDesktopLinuxEngine; check if the path is correct and if the daemon is running: open //./pipe/dockerDesktopLinuxEngine: The system cannot find the file specified.
```

On resume, only the stopped development PostgreSQL container was started. Controller logic was not changed. The new cluster `smp-sprint1-0d0c1b96` completed final-source API/Fabric, security inspection and cleanup. The independent six-scenario atomic evidence is preserved; it tests the unchanged candidate, rather than being presented as a fresh rerun.

Evidence under the repository's ignored `.logs` directory: `sprint2-atomic-kind.json`, `sprint2-independent-ack.json`, `sprint2-qualification.json`, `sprint2-api-kind.json`, `sprint2-postgres.jsonl` (fresh PASS), `sprint2-postgres-before-resume-failure.jsonl` (preserved failure), `sprint2-go-final.jsonl`, `sprint2-vet-final.log`, `sprint2-postgres-tls-final.log`, `sprint2-real-okta-status.json` and consolidated `sprint2-evidence.json`. Console evidence is in its `.logs/sprint2-unit.log`, `.logs/sprint2-build.log` and `.logs/sprint2-format.log`.

## Resume from normal Windows PowerShell

Start Docker Desktop and wait for its Linux engine. Then execute:

```powershell
Set-Location E:\Sathonate\fabric-control-plane
docker version --format '{{.Server.Version}}'
if ($LASTEXITCODE -ne 0) { throw 'Docker unavailable; stop runtime qualification' }
python tools/sprint2/postgres.py
if ($LASTEXITCODE -ne 0) { throw 'PostgreSQL authorization qualification failed' }
$env:SMP_QUALIFICATION_PREFIX = 'sprint2'
$env:SMP_QUALIFICATION_WORKER = 'sathonate/fabric-worker:sprint2'
$env:SMP_QUALIFICATION_CALICO = '1'
$env:SMP_SECURE_OIDC_FIXTURE = '1'
python tools/sprint1/qualify.py
if ($LASTEXITCODE -ne 0) { throw 'Disposable runtime qualification failed' }
```

These runtime commands have now passed on the resumed final source. For another run, refresh source/build provenance and verify the worker image matches the source. Run shared-database fixtures serially with the local qualification API stopped. Failed evidence stays separate. The duplicated username has been corrected in a fresh PKCE flow. The user then reached required passkey enrollment and explicitly deferred further Okta work. Resume only after the owner resolves the enrollment requirement without an application bypass; verify the real callback and issuer/subject before creating the previously approved read-only development membership. The local starting URL is `http://127.0.0.1:5173/`; choose **Continue with Okta** for a fresh flow after diagnosis, then complete every required security method. Application-side bypass is not an alternative. Sprint 3 remains unstarted. Consult `sprint2-incident-migration.md` for exact historical-remediation approvals and unsupported rollback boundaries.
