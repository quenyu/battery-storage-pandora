param([ValidateRange(1024,65535)][int]$DatabasePort = 55432, [string]$HttpAddress = '127.0.0.1:8080', [string]$Token = 'local-mvp-test-token')
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
Push-Location $taskRoot
try {
    & "$PSScriptRoot/postgres-setup.ps1" -Port $DatabasePort
    $env:DATABASE_URL = "postgres://pandora_owner@127.0.0.1:${DatabasePort}/pandora_mvp_demo?sslmode=disable"
    & go run ./cmd/pandora migrate
    if ($LASTEXITCODE -ne 0) { throw 'Migration failed.' }
    & "$PSScriptRoot/postgres-grant.ps1" -Port $DatabasePort -Database pandora_mvp_demo
    $env:DATABASE_URL = "postgres://pandora_app@127.0.0.1:${DatabasePort}/pandora_mvp_demo?sslmode=disable"
    $env:HTTP_ADDR = $HttpAddress
    $env:DEV_MODE = 'true'
    $env:DEV_API_TOKEN = $Token
    Write-Host "Swagger UI: http://$HttpAddress/swagger/ (Ctrl+C to stop the API). Use -Token value in Authorize."
    & go run ./cmd/pandora serve
    if ($LASTEXITCODE -ne 0) { throw 'Service stopped with an error.' }
} finally { Pop-Location }
