param([string]$Base = 'http://127.0.0.1:8080/api/v1')
$ErrorActionPreference = 'Stop'
$run = [guid]::NewGuid().ToString('N')
function Send-Command {
    param([string]$Path, [hashtable]$Body, [string]$Key = ([guid]::NewGuid().ToString()), [string]$Method = 'POST')
    Invoke-RestMethod -Method $Method -Uri "$Base$Path" -ContentType 'application/json; charset=utf-8' `
        -Headers @{'Idempotency-Key'=$Key} -Body ([Text.Encoding]::UTF8.GetBytes(($Body | ConvertTo-Json)))
}
function Check([bool]$Condition, [string]$Message) { if (!$Condition) { throw $Message } }
$aliceCode = "0000-A-$run"
$bobCode = "0000-B-$run"
$alice = Send-Command '/employees' @{name='Иван Петров';barcode=$aliceCode}
$bob = Send-Command '/employees' @{name='Ольга Иванова';barcode=$bobCode}
$number = Get-Random -Minimum 1000 -Maximum 1000000000
$cabinet = Send-Command '/cabinets' @{number=$number}
$shelf = Send-Command "/cabinets/$($cabinet.id)/shelves" @{number=1}
$cells = @(1..3 | ForEach-Object { Send-Command "/shelves/$($shelf.id)/cells" @{number=$_} })
$registered = Send-Command '/batteries' @{inventory_code="AKB-$run";cell_id=$cells[0].id;employee_barcode=$aliceCode}
$id = $registered.battery.id
$key = [guid]::NewGuid().ToString()
$issued = Send-Command "/batteries/$id/checkout" @{employee_barcode=$aliceCode} -Key $key
$returned = Send-Command "/batteries/$id/return" @{employee_barcode=$bobCode;target_cell_id=$cells[1].id}
Check ($returned.operation.actor_employee_id -eq $bob.id) 'Wrong return actor'
Check ($returned.operation.from_holder_employee_id -eq $alice.id) 'Wrong previous holder'
$replay = Send-Command "/batteries/$id/checkout" @{employee_barcode=$aliceCode} -Key $key
Check ($replay.operation.id -eq $issued.operation.id) 'Replay created a new operation'
$current = Invoke-RestMethod "$Base/batteries/$id"
Check ($current.status -eq 'stored') 'Replay changed current state'
$moved = Send-Command "/batteries/$id/move" @{employee_barcode=$aliceCode;target_cell_id=$cells[2].id}
$replacement = Send-Command "/employees/$($alice.id)/credential" @{barcode="NEW-$run"} -Method PUT
$lost = Send-Command "/batteries/$id/loss" @{employee_barcode=$replacement.barcode;reason='Не найдена при проверке'}
Check ($lost.battery.status -eq 'lost' -and $null -eq $lost.battery.cell_id) 'Wrong loss state'
Check ($lost.operation.from_cell_id -eq $cells[2].id) 'Last location was lost'
$history = Invoke-RestMethod "$Base/operations?battery_id=$id"
Check ($history.items.Count -eq 5 -and $lost.battery.version -eq 5) 'Wrong history/version'
[pscustomobject]@{battery_id=$id;status=$lost.battery.status;operations=$history.items.Count;last_address=$moved.battery.cell_id;swagger=($Base -replace '/api/v1$','/swagger/')} | Format-List
