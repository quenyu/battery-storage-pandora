param([ValidateRange(1024,65535)][int]$DatabasePort = 55432, [string]$HttpAddress = '127.0.0.1:18080', [switch]$Reset)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location $projectRoot
try {
    & "$PSScriptRoot/postgres-setup.ps1" -Port $DatabasePort
    if ($Reset) {
        # Local development data only: recreate empty databases for the current baseline schema.
        $psql = Join-Path $projectRoot '.local/pgsql/bin/psql.exe'
        foreach ($script in 'postgres-reset.sql', 'postgres-init.sql') {
            & $psql -X -h 127.0.0.1 -p $DatabasePort -U postgres -d postgres -v ON_ERROR_STOP=1 -f (Join-Path $PSScriptRoot $script)
            if ($LASTEXITCODE -ne 0) { throw "Database reset failed in $script." }
        }
    }
    $env:DATABASE_URL = "postgres://pandora_owner@127.0.0.1:${DatabasePort}/pandora_storage?sslmode=disable"
    & go run ./cmd/migrate
    if ($LASTEXITCODE -ne 0) { throw 'Migration failed.' }
    & "$PSScriptRoot/postgres-grant.ps1" -Port $DatabasePort -Database pandora_storage
    $env:DATABASE_URL = "postgres://pandora_app@127.0.0.1:${DatabasePort}/pandora_storage?sslmode=disable"
    $env:HTTP_ADDR = $HttpAddress
    Write-Host "Swagger: http://$HttpAddress/swagger/ (Ctrl+C to stop)"
    & go run ./cmd/pandora
    if ($LASTEXITCODE -ne 0) { throw 'Service stopped with an error.' }
} finally { Pop-Location }
