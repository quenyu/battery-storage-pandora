param([ValidateRange(1024,65535)][int]$Port = 55432)
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
$localDir = Join-Path $taskRoot '.local'
$pgRoot = Join-Path $localDir 'pgsql'
$dataDir = Join-Path $localDir 'pgdata'
$archivePath = Join-Path $localDir 'postgresql-17.11-1-windows-x64-binaries.zip'
$downloadUrl = 'https://get.enterprisedb.com/postgresql/postgresql-17.11-1-windows-x64-binaries.zip'
# Digest pins the exact archive downloaded over TLS from the official EDB host.
$expectedHash = '6EABDF00D2893713B75DB4336A23C3FDF505F056E217EC6E2E95D901750CFEA3'
New-Item -ItemType Directory -Force $localDir | Out-Null
if (!(Test-Path (Join-Path $pgRoot '.runtime-ready'))) {
    if (!(Test-Path $archivePath)) {
        Write-Host 'Downloading PostgreSQL 17.11 portable binaries (about 325 MiB)...'
        & curl.exe -fL --retry 3 --output $archivePath $downloadUrl
        if ($LASTEXITCODE -ne 0) { throw 'PostgreSQL download failed.' }
    }
    if ((Get-FileHash $archivePath -Algorithm SHA256).Hash -ne $expectedHash) {
        throw "Archive checksum mismatch. Remove only $archivePath and retry."
    }
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $archive = [IO.Compression.ZipFile]::OpenRead($archivePath)
    try {
        foreach ($entry in $archive.Entries) {
            # No pgAdmin/StackBuilder: only PostgreSQL runtime and its licenses.
            if ($entry.FullName -notmatch '^pgsql/(bin/|lib/|share/|[^/]+\.txt$)') { continue }
            if (!$entry.Name) { continue }
            $destination = [IO.Path]::GetFullPath((Join-Path $localDir $entry.FullName))
            if (!$destination.StartsWith([IO.Path]::GetFullPath($pgRoot) + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
                throw 'Archive contains an invalid path.'
            }
            New-Item -ItemType Directory -Force (Split-Path -Parent $destination) | Out-Null
            [IO.Compression.ZipFileExtensions]::ExtractToFile($entry, $destination, $true)
        }
    } finally { $archive.Dispose() }
    Set-Content -Encoding ascii (Join-Path $pgRoot '.runtime-ready') $expectedHash
}
if (!(Test-Path (Join-Path $dataDir 'PG_VERSION'))) {
    & (Join-Path $pgRoot 'bin/initdb.exe') -D $dataDir -U postgres --encoding=UTF8 --locale=C --auth=trust
    if ($LASTEXITCODE -ne 0) { throw 'PostgreSQL initdb failed.' }
    # Explicit IPv4 loopback only. No system service or system configuration changes.
    @'
host all all 127.0.0.1/32 trust
'@ | Set-Content -Encoding ascii (Join-Path $dataDir 'pg_hba.conf')
}
& (Join-Path $PSScriptRoot 'postgres-start.ps1') -Port $Port
& (Join-Path $pgRoot 'bin/psql.exe') -X -h 127.0.0.1 -p $Port -U postgres -d postgres -v ON_ERROR_STOP=1 -f (Join-Path $PSScriptRoot 'postgres-init.sql')
if ($LASTEXITCODE -ne 0) { throw 'Database/role setup failed.' }
Write-Host "Owner: postgres://pandora_owner@127.0.0.1:${Port}/pandora_storage?sslmode=disable"
Write-Host "App:   postgres://pandora_app@127.0.0.1:${Port}/pandora_storage?sslmode=disable"
Write-Host 'Run migrations as owner, then scripts/postgres-grant.ps1 for each migrated database.'
