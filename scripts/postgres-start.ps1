param([ValidateRange(1024,65535)][int]$Port = 55432)
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
$pgRoot = Join-Path $taskRoot '.local/pgsql'
$dataDir = Join-Path $taskRoot '.local/pgdata'
$pgCtl = Join-Path $pgRoot 'bin/pg_ctl.exe'
if (!(Test-Path $pgCtl) -or !(Test-Path (Join-Path $dataDir 'PG_VERSION'))) {
    throw 'Run scripts/postgres-setup.ps1 first.'
}
& $pgCtl -D $dataDir status *> $null
if ($LASTEXITCODE -ne 0) {
    $logPath = Join-Path $taskRoot '.local/postgres.log'
    $pgArguments = '-D "{0}" -l "{1}" -o "-h 127.0.0.1 -p {2}" -w start' -f $dataDir, $logPath, $Port
    $process = Start-Process -FilePath $pgCtl -ArgumentList $pgArguments -WindowStyle Hidden -PassThru
    # Start-Process -Wait waits for the entire descendant tree (including the
    # database daemon). Wait only for pg_ctl's readiness/exit result instead.
    $process.WaitForExit()
    if ($process.ExitCode -ne 0) { throw "PostgreSQL failed to start. See $logPath" }
}
& (Join-Path $pgRoot 'bin/pg_isready.exe') -h 127.0.0.1 -p $Port -U postgres
if ($LASTEXITCODE -ne 0) { throw 'PostgreSQL readiness check failed.' }
