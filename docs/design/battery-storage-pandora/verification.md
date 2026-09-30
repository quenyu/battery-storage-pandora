# Демонстрация и проверки

Дата: 28.09.2026. Контракт: [openapi.yaml](openapi.yaml). Модель: [architecture.md](architecture.md). Этот документ описывает приёмку будущего Go-приложения; сервис сейчас не запущен и не реализован.

## 1. Что проверено на этапе проектирования

- OpenAPI 3.0.3 проверен `openapi-spec-validator`: структурных ошибок нет.
- 36 примеров запросов, успешных ответов и ошибок проверены против схем через `openapi-schema-validator` (OAS30).
- Проект SQL разобран PostgreSQL-парсером `pglast`: 22 SQL-выражения синтаксически корректны.
- Локальные ссылки Markdown и `$ref` контракта проверены: отсутствующих целей нет.
- Проверена уникальность 19 operationId, наличие Idempotency-Key у всех 10 изменяющих маршрутов и согласованность состояния/версии/журнала в пяти примерах учётных команд.

Контракт содержит 14 путей, 19 HTTP-операций и 23 схемы. Примеры отдельных маршрутов независимы; это не последовательность состояний одной тестовой БД.

**Не проверено исполнением:** применение DDL к реальному PostgreSQL, планы запросов, права DB-роли, транзакции, конкурентность, HTTP-обработчики и идемпотентность. Swagger UI пока не реализован, браузерные проверки ещё не выполнены. Локальный `psql` при проверке не найден; парсер подтверждает синтаксис, но не заменяет запуск SQL. Эти проверки обязательны при реализации.

## 2. Минимальная демонстрация

После реализации запустить сервис локально и создать чистую тестовую БД по миграциям. Использовать вымышленных сотрудников; аппаратуры нет. Приведённый PowerShell — сценарий для будущего сервиса, сейчас он не выполнялся.

```powershell
$base = 'http://127.0.0.1:8080/api/v1'

function Send-DemoCommand {
    param([string]$Path, [hashtable]$Body,
          [string]$Key = ([guid]::NewGuid().ToString()),
          [string]$Method = 'POST')
    Invoke-RestMethod -Method $Method -Uri "$base$Path" `
        -ContentType 'application/json; charset=utf-8' `
        -Headers @{ 'Idempotency-Key' = $Key } `
        -Body ([System.Text.Encoding]::UTF8.GetBytes(($Body | ConvertTo-Json -Depth 10)))
}

$ivan = Send-DemoCommand '/employees' @{ name = 'Иван Петров'; barcode = '00001234' }
$olga = Send-DemoCommand '/employees' @{ name = 'Ольга Иванова'; barcode = '00009876' }
$cab1 = Send-DemoCommand '/cabinets' @{ number = 1 }
$cab2 = Send-DemoCommand '/cabinets' @{ number = 2 }
$s11 = Send-DemoCommand "/cabinets/$($cab1.id)/shelves" @{ number = 1 }
$s12 = Send-DemoCommand "/cabinets/$($cab1.id)/shelves" @{ number = 2 }
$s21 = Send-DemoCommand "/cabinets/$($cab2.id)/shelves" @{ number = 1 }
$c111 = Send-DemoCommand "/shelves/$($s11.id)/cells" @{ number = 1 }
$c112 = Send-DemoCommand "/shelves/$($s11.id)/cells" @{ number = 2 }
$c121 = Send-DemoCommand "/shelves/$($s12.id)/cells" @{ number = 1 }
$c211 = Send-DemoCommand "/shelves/$($s21.id)/cells" @{ number = 1 }

$registered = Send-DemoCommand '/batteries' @{
    inventory_code = 'AKB-0001'; cell_id = $c111.id; employee_barcode = '00001234'
}
$second = Send-DemoCommand '/batteries' @{
    inventory_code = 'AKB-0002'; cell_id = $c112.id; employee_barcode = '00009876'
}
$batteryId = $registered.battery.id
$checkoutKey = [guid]::NewGuid().ToString()
$checkout = Send-DemoCommand "/batteries/$batteryId/checkout" @{
    employee_barcode = '00001234'
} -Key $checkoutKey
$replay = Send-DemoCommand "/batteries/$batteryId/checkout" @{
    employee_barcode = '00001234'
} -Key $checkoutKey
# У checkout и replay совпадают operation.id, battery.version и JSON-значения ответа.

$returned = Send-DemoCommand "/batteries/$batteryId/return" @{
    employee_barcode = '00009876'; target_cell_id = $c121.id
}
# Автор — Ольга, предыдущий держатель — Иван; теперь место 1.2.1.
$moved = Send-DemoCommand "/batteries/$batteryId/move" @{
    employee_barcode = '00001234'; target_cell_id = $c211.id
}
$lost = Send-DemoCommand "/batteries/$batteryId/loss" @{
    employee_barcode = '00001234'; reason = 'Не обнаружена при проверке'
}
$newCredential = Send-DemoCommand "/employees/$($ivan.id)/credential" @{
    barcode = '00005678'
} -Method 'PUT'

Invoke-RestMethod "$base/batteries/$batteryId"
Invoke-RestMethod "$base/operations/$($lost.operation.id)"
Invoke-RestMethod "$base/operations?battery_id=$batteryId"
Invoke-RestMethod "$base/operations?employee_id=$($ivan.id)"
Invoke-RestMethod "$base/operations?cell_id=$($c211.id)"
```

Ожидается: у AKB-0001 версия 5 и статус lost, текущие cell_id/holder_employee_id null. В истории ровно register → checkout → return → move → loss; повтор checkout не добавил строку. Последняя операция хранит from_cell_id ячейки 2.1.1. AKB-0002 остаётся в 1.1.2. Старые операции Ивана ссылаются на прежнюю credential, его UUID не меняется.

### Демонстрация через Swagger UI

После реализации открыть `http://127.0.0.1:8080/swagger/`. UI должен загрузить контракт с `/openapi.yaml` и показать все бизнес-операции. Через Try it out выполнить `POST /employees` с тестовыми именем и штрихкодом и новым Idempotency-Key; повторить тот же запрос с тем же ключом и убедиться, что сотрудник не создаётся второй раз. Затем через `GET /employees/{employee_id}` получить созданную запись. Для остальных действий можно использовать UI или PowerShell выше.

## 3. Приёмочные проверки

| ID / требование | Подготовка и действие | Проверяемый результат |
|---|---|---|
| T01 / SC-01, SC-09 | Создать сотрудника со штрихкодом `00001234`, заменить на `00005678` | Ведущие нули сохранены. Два credential, только один действующий; новый код определяет того же сотрудника |
| T02 / SC-02 | Шкаф 1 имеет две полки, шкаф 2 — одну; у полок разное число ячеек | Иерархия корректна, локальные номера могут повторяться в разных родителях |
| T03 / REQ-01 | Два последовательных register в одну ячейку с разными кодами АКБ | Второй — 409 CELL_OCCUPIED; нет второй АКБ, операции и успешного ключа |
| T04 / SC-04–06 | Выдать, вернуть другим сотрудником в другое место, переместить | Каждая операция меняет версию на 1; автор и держатель отражают свои роли в движении |
| T05 / REQ-04 | На реальной PostgreSQL одновременно выполнить checkout одной stored АКБ разными сотрудниками и ключами | Один 201, другой 409 INVALID_TRANSITION; одна выдача, один держатель |
| T06 / REQ-01, REQ-04 | Одновременно переместить две разные АКБ в одну свободную ячейку | Один 201, другой 409 CELL_OCCUPIED; проигравшая АКБ осталась на исходном месте |
| T07 / REQ-03 | Одновременно два одинаковых запроса с одним Idempotency-Key | Один эффект в БД, идентичные JSON-результаты и HTTP-статусы |
| T08 / REQ-03 | После checkout и последующего return повторить исходный checkout с прежним ключом | Возвращён старый результат checkout без новой выдачи; GET показывает актуальный stored |
| T09 / REQ-03 | Успешный ключ повторить с другим телом или маршрутом | 409 IDEMPOTENCY_CONFLICT; состояние не изменилось |
| T10 / REQ-02, REQ-06 | В тесте вызвать сбой после UPDATE batteries, до INSERT operations или записи ответа | Полный rollback: версия, место, журнал и запись ключа не изменились |
| T11 / REQ-03 | Потерять HTTP-ответ после COMMIT, повторить ключ | Возвращён сохранённый результат; повторного движения нет |
| T12 / SC-07, REQ-06 | Отметить потерю отдельно из stored и из issued | В обоих случаях lost, текущие ссылки null; соответственно from_cell_id или from_holder_employee_id сохранён |
| T13 / REQ-05 | После замены карты прочитать историю по UUID сотрудника | Старые операции находятся. Новая команда со старым кодом — 404 EMPLOYEE_NOT_FOUND |
| T14 / SC-09, REQ-03 | После замены карты повторить старую успешную команду с её ключом | Старый ответ возвращён: проверка повтора выполняется до проверки отзыва карты |
| T15 / SC-09 | Попытаться назначить исторический/чужой штрихкод | 409 BARCODE_EXISTS; текущий действующий код не отозван из-за неудачной замены |
| T16 / REQ-06 | Выполнить move для issued, return для stored, checkout для lost, move в ту же ячейку | 409 INVALID_TRANSITION, без новых операций |
| T17 / SC-08 | Фильтр employee_id совпадает сразу с автором и держателем одной операции | Строка возвращается один раз; from включительно, to исключительно |
| T18 / контракт | Неизвестное поле, дубликат JSON-поля, null, неверный UUID, отрицательный номер, пустая причина | 400 VALIDATION_ERROR, без изменений |
| T19 / контракт | Нет ключа POST/PUT, неверный Content-Type, тело >64 KiB | Соответственно 400, 415 и 413 |
| T20 / DDL | Применить SQL к чистой PostgreSQL; попытаться напрямую записать две АКБ в ячейку и issued без holder | Ограничения БД отклоняют оба нарушения; валидные состояния принимаются |
| T21 / REQ-07 | Открыть `/swagger` и `/swagger/` в браузере после запуска Go-сервиса | Редирект и UI работают; все 19 операций, схемы и примеры доступны, нет ошибок загрузки контракта |
| T22 / REQ-07 | Через Try it out создать сотрудника, повторить ключ, прочитать сотрудника | Реальные запросы идут в `/api/v1` того же origin; заголовок и JSON отправляются; нет второй записи |
| T23 / REQ-07 | Заблокировать внешнюю сеть в браузерной проверке, сохранив localhost | UI, JS/CSS, контракт и Try it out продолжают работать без CDN или внешнего валидатора |
| T24 / REQ-08 | Прочитать `/openapi.yaml` и сравнить с исходным файлом проекта | Содержимое совпадает; проверка OpenAPI проходит; UI не показывает устаревшую копию |

Для T05–T07 использовать два независимых соединения PostgreSQL и синхронный старт команд; один последовательный тест или mock не доказывает конкурентную корректность. T10 проверяется интеграционным тестом с управляемым сбоем внутри транзакции. T20 отдельно проверяет миграцию; это не заменяется парсингом SQL.

## 4. Порядок следующего этапа

1. Создать миграцию на основе schema.sql и проверить её в чистой тестовой PostgreSQL (T20).
2. Реализовать справочники, создание сотрудника и замену штрихкода (T01–T02, T15).
3. Реализовать транзакции учёта и механизм повторов вместе, затем основные чтения (T03–T14, T16–T17).
4. Добавить HTTP-валидацию и преобразование ошибок согласно OpenAPI (T18–T19).
5. Подключить Swagger UI и выдачу OpenAPI тем же Go-сервером, выполнить браузерные проверки T21–T24.
6. Выполнить демонстрацию, проверить полный набор тестов, описать запуск и адрес Swagger UI в README. Приложение слушает localhost; внешнее развёртывание не требуется.

SQL, документация и OpenAPI должны обновляться согласованно при изменении контракта. Обнаружение потерянной АКБ, удаление, роли и интеграции не добавляются незаметно в реализацию v1.
