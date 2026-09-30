param([string]$Base = 'http://127.0.0.1:8080/api/v1', [string]$Token = $env:DEV_API_TOKEN)
$ErrorActionPreference = 'Stop'
if (!$Token) { throw 'Set DEV_API_TOKEN or pass -Token.' }
$baseUri = [Uri]$Base
if ($baseUri.Host -notin @('127.0.0.1','localhost','[::1]')) { throw 'Demo is local-only.' }
$run = [guid]::NewGuid().ToString('N')
function Send-Command {
    param([string]$Path, [hashtable]$Body, [string]$Key = ([guid]::NewGuid().ToString()), [string]$Method = 'POST')
    Invoke-RestMethod -Method $Method -Uri "$Base$Path" -ContentType 'application/json; charset=utf-8' -Headers @{'Idempotency-Key'=$Key;Authorization="Bearer $Token"} -Body ([Text.Encoding]::UTF8.GetBytes(($Body | ConvertTo-Json)))
}
function Check([bool]$Condition, [string]$Message) { if (!$Condition) { throw $Message } }
$card = "0000-demo-$run"
$ivan = Send-Command '/employees' @{display_name='Тестовый сотрудник Иван';personnel_number="demo-$run"}
$credential = Send-Command "/employees/$($ivan.id)/credentials" @{value=$card}
$number = Get-Random -Minimum 100000 -Maximum 1000000000
$registered = Send-Command '/batteries' @{inventory_code="DEMO-$run";destination_location="$number.1.1";actor_credential_value=$card}
$id = $registered.battery.id
$key = [guid]::NewGuid().ToString()
$issued = Send-Command "/batteries/$id/take" @{actor_credential_value=$card;expected_version=1} -Key $key
$newCard = "$card-new"
$newCredential = Send-Command "/employees/$($ivan.id)/credentials" @{value=$newCard;replaces_credential_id=$credential.id}
$returned = Send-Command "/batteries/$id/return" @{actor_credential_value=$newCard;destination_location="$number.1.2";expected_version=2}
$moved = Send-Command "/batteries/$id/move" @{actor_credential_value=$newCard;destination_location="$number.1.3";observed_source_location="$number.1.2";expected_version=3}
$replay = Send-Command "/batteries/$id/take" @{actor_credential_value=$card;expected_version=1} -Key $key
Check ($replay.operation.id -eq $issued.operation.id) 'Replay created another operation'
Check ($returned.operation.actor_employee_id -eq $ivan.id) 'Card replacement changed employee identity'
Check ($moved.battery.version -eq 4) 'Wrong version'
$history = Invoke-RestMethod -Uri "$Base/batteries/$id/operations" -Headers @{Authorization="Bearer $Token"}
Check ($history.items.Count -eq 4) 'Wrong history count'
$history.items | Select-Object battery_version,type,source_location,destination_location,actor_employee_id | Format-Table
Write-Host "Demo verified. Employee=$($ivan.id), battery=$id, current=$($moved.battery.current_location)"
