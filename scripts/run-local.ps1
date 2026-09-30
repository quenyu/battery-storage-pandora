param([ValidateRange(1024,65535)][int]$DatabasePort = 55432, [string]$HttpAddress = '127.0.0.1:18080')
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location $projectRoot
try {
    & "$PSScriptRoot/postgres-setup.ps1" -Port $DatabasePort
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
