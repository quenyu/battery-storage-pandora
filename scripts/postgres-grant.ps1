param(
    [ValidateRange(1024,65535)][int]$Port = 55432,
    [ValidateSet('pandora', 'pandora_test')][string]$Database = 'pandora'
)
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
$psql = Join-Path $taskRoot '.local/pgsql/bin/psql.exe'
& $psql -X -h 127.0.0.1 -p $Port -U pandora_owner -d $Database -v ON_ERROR_STOP=1 -f (Join-Path $PSScriptRoot 'postgres-grants.sql')
if ($LASTEXITCODE -ne 0) { throw 'Application grants failed; run migrations as pandora_owner first.' }

