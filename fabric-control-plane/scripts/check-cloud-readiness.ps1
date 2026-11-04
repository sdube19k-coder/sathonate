param([string]$BaseUrl='http://127.0.0.1:8080',[switch]$Probe)
$ErrorActionPreference='Stop'
$uri=[uri]$BaseUrl
if(-not $uri.IsAbsoluteUri -or -not $uri.IsLoopback -or $uri.UserInfo -or $uri.Query -or $uri.Fragment -or $uri.AbsolutePath -ne '/' -or $uri.Scheme -notin @('http','https')){throw 'Use the explicit loopback API origin for local prelaunch administration'}
$endpoint=$BaseUrl.TrimEnd('/')+'/api/v1/readiness'
if($Probe){$report=Invoke-RestMethod -Method Post -Uri ($endpoint+'/probe') -ContentType 'application/json' -Body '{}' -TimeoutSec 90}
else{$report=Invoke-RestMethod -Uri $endpoint -TimeoutSec 15}
$report.checks | Select-Object group,title,status,required | Format-Table -Wrap -AutoSize
Write-Output "Cloud execution enabled: $($report.executionEnabled). Qualified: $($report.ready). Read-only probe: $($report.probed)."
if(-not $report.ready){Write-Output 'Resolve the reported prerequisites and production qualification gates. This command never provisions resources.'}
