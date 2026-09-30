$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
$pgCtl = Join-Path $taskRoot '.local/pgsql/bin/pg_ctl.exe'
$dataDir = Join-Path $taskRoot '.local/pgdata'
if (!(Test-Path $pgCtl)) { throw 'PostgreSQL has not been set up.' }
& $pgCtl -D $dataDir status *> $null
if ($LASTEXITCODE -eq 0) {
    & $pgCtl -D $dataDir -m fast -w stop
    if ($LASTEXITCODE -ne 0) { throw 'PostgreSQL failed to stop.' }
} else { Write-Host 'PostgreSQL is already stopped.' }

