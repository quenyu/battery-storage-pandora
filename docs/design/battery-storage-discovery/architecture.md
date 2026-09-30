# Архитектура v2: текстовые места

Обновлено 30.09.2026 по решению руководителя, переданному пользователем. Отдельные таблицы шкафов/полок/ячеек удалены из проекта; адреса в журнале — текст. Одна АКБ на место и RETURN только взявшим сотрудником сохраняются. Рабочий Go backend и его миграции не меняются.

## Граница и компоненты

```text
ТСД ──┐
      ├── HTTP/REST v1 → Go backend → PostgreSQL
Web ──┘                 └─ OpenAPI + Swagger UI
```

Go владеет переходами и правами, PostgreSQL — current/history/result. Клиенты не пишут SQL напрямую. Система оформляет движение АКБ, но не ведёт конфигурацию/active состояние физических мест. Управление замками, датчики, корпоративные интеграции и детальный клиент ТСД/offline остаются вне подтверждённого этапа.

## Решения и история пересмотра

| ID | Статус | Решение и пересмотр |
|---|---|---|
| D01 | Предложено, сохраняется | Простой монолит, одна транзакция; Kafka/Redis/ES/CQRS framework не нужны |
| D02 | Предложено, обновлено D09 | Current в batteries + immutable history; current_cell_id заменён current_location TEXT |
| D03 | Предложено, сохраняется | TEXT+CHECK типов, новые переходы через согласованный DDL/Go/OpenAPI |
| D04 | Предложено, сохраняется | UUID v4 из Go, TIMESTAMPTZ, порядок одной АКБ по version |
| D05 | Предложено, обновлено D09 | Battery FOR UPDATE; занятость текста через UNIQUE; destination cell-row больше нет |
| D06 | Предложено, сохраняется | Idempotency-Key и техническая таблица первоначального ответа |
| D07 | Заменено | Прежняя devices-table заменена nullable device_code в операции; метка достаточна для трассировки, lifecycle устройств не нужен в этом этапе |
| D08 | Предложено, обновлено | Явные TAKE/RETURN/MOVE, текстовые DTO; удалены CRUD мест/устройств |
| D09 | Принято из уточнения пользователя | Руководитель попросил хранить места текстом в battery_operations, без отдельных таблиц. Прежняя нормализация заменена для согласованного этапа журнала. Пересмотр — при требовании существования/пустых/отключённых мест или конфигурации шкафа |

D09 заменяет первоначальное правило «не хранить место строкой». Это подтверждение формы хранения мест, а не утверждение всей технической архитектуры или API. Предыдущее решение оставлено в этой истории и документах старой демонстрации.

## Current и history

| Подход | Оценка для нашей АКБ |
|---|---|
| Current location/status/holder/version в batteries | Выбран: одна строка GET/lock, простой UNIQUE занятого адреса |
| Отдельная battery_state | Пока дополнительный join и риск отсутствующей projection; полезно при независимом сложном lifecycle |
| Вычисление последнего события на каждый запрос | Полезно для сверки, но усложняет список занятых мест и конкурентную защиту |

Source_location/destination_location в operations выполняют указание руководителя. Current_location в batteries — техническая проекция для быстрого состояния и занятости; она не возвращает справочник мест. Дублирование намеренное, одна транзакция и deferred guard защищают совпадение. Source сервер берёт из заблокированного current; при MOVE сохраняются оба адреса, при TAKE source+holder, при RETURN прежний holder+destination.

Текстовый snapshot сохраняет прошлое написание. Переименование физического места не переписывает журнал; без устойчивого ID места автоматической связи старого/нового названия нет. Правила канонизации Q30 нужно согласовать: `1.1.1` и `01.1.1` пока разные значения. Предложены непустой TEXT COLLATE C и запрет крайних обычных пробелов; числовой regex не утверждён.

## Транзакция и конкурентность

READ COMMITTED, HTTP success после COMMIT:

1. Проверить trusted API principal/route permission, DTO, Idempotency-Key и canonical hash.
2. BEGIN, reserve(scope,key) INSERT ON CONFLICT DO NOTHING. Для существующего key — отдельный SELECT hash/result; replay без новой команды.
3. SAVEPOINT business; actor по credential, employee/credential FOR SHARE; повторно проверить activity/сопоставление после блокировки.
4. Для существующей АКБ SELECT из batteries FOR UPDATE без join к журналу. После ожидания проверить актуальные status/version/source/holder. Для создания реестра — UNIQUE inventory_code.
5. Проверить переход, expected_version/observed_source_location если переданы, RETURN actor=current holder, MOVE destination≠source. Destination — текст, location-row не существует.
6. Записать event и current/version с общим timestamp. Partial UNIQUE current_location WHERE NOT NULL окончательно защищает занятость; предварительный SELECT только ранняя диагностика.
7. Сохранить status/body ключа; COMMIT; отдать результат. Deferred guards проверяют current/history и complete request.

Два TAKE блокируются одной battery-row: второй после COMMIT видит ISSUED и получает 409. Поведение FOR UPDATE — [PostgreSQL](https://www.postgresql.org/docs/17/explicit-locking.html#LOCKING-ROWS).

Две разные АКБ на один адрес не имеют общей location-row, но UNIQUE index не допускает обе записи. Победитель COMMIT, проигравший получает 23505 по конкретному `batteries_one_per_location_uq`, который Go превращает в 409 LOCATION_OCCUPIED. [Уникальные индексы PostgreSQL](https://www.postgresql.org/docs/17/indexes-unique.html) защищают одинаковые значения; физическую эквивалентность разных строк должна обеспечивать канонизация.

После SQL violation транзакция aborted: ROLLBACK TO SAVEPOINT business, сохранить кэшируемый конфликт, COMMIT без движения. Event, projection, освобождение source проигравшей команды тоже откатываются. Нельзя полагаться только на предварительное чтение «свободно». Встречные MOVE в занятые места могут привести к deadlock; 40P01/lock timeout — rollback всей транзакции и bounded retry либо 503 с прежним ключом. Обмен занятых мест не является отдельной операцией MVP.

Порядок locks: request key → employee → credential → battery. Деактивация employee FOR UPDATE, затем чтение outstanding custody без других battery locks. Отзыв credential использует конфликтующую с FOR SHARE блокировку. Нет locks шкафов/полок/ячеек. Точные таймауты и retry budgets уточнить по нагрузке, не выдумывать SLO.

## Идемпотентность и безопасность

UUID key сохраняется клиентом до первого запроса. Scope — stable trusted API principal, не карточка/произвольная метка ТСД. SHA256 включает method, canonical resource path, нормализованный DTO; JSON order/UUID case не меняют hash, credential/inventory/location сравниваются точно. Bearer token не включается в hash.

Reserve/result и движение одна транзакция. Incomplete result нельзя COMMIT. PK(scope,key) синхронизирует одинаковые parallel запросы; после конфликтующего INSERT — отдельный SELECT с новым snapshot. Другой route/body с ключом даёт 409 IDEMPOTENCY_KEY_REUSED. Кэш успеха и deterministic business404/409/422; auth/parse/500/503 не кэшируются. Исправленный запрос — новый ключ. Replay отдаёт исходный status/JSON, даже если later RETURN уже изменил current; свежий state через GET. После отзыва карты replay только тому же principal с сохранёнными текущими правами; отозванный bearer не получает response.

Result retention без автоудаления до определения retry horizon. Requests нужна и для employees/credentials, которые не создают battery event; UNIQUE request_key в истории не сохраняет ответы всех команд. Гарантия — один эффект в БД на scope/key, не ровно одна HTTP delivery/физическое действие.

Карта идентифицирует actor, trusted API context определяет права. Bearer в OpenAPI — предложение интерфейса, issuer/JWT/OIDC/LDAP не подтверждены (Q19/Q25). Device_code server назначает из известного доверенного контекста интеграции, иначе NULL; это метка трассировки, не аппаратная authentication. Devices CRUD отсутствует. Права на шкафы при текстовом адресе потребуют отдельного согласованного источника policy, а не выдуманного registry.

## Go и модели

Go/pgx/явный SQL, без ORM. cmd — запуск; domain — переходы/ошибки; service — сценарий/права/транзакция; repository — SQL/locks/reads; transport/http — decode/validate/status; migrations — согласованный DDL; docs — контракт. Repository не делает COMMIT на каждый INSERT. Не вводить интерфейсы/слои без ответственности; существующий internal/api сейчас не переносится.

TEXT+CHECK выбран вместо enum (хорошая типизация, но миграция/откат набора сложнее) и reference operation_types (metadata полезна, однако новая строка не реализует переход). Nullable endpoints ограничены shape CHECK; subtype tables/JSON-only payload усложнят журнал. UUID поддерживает стиль проекта и не имеет JavaScript numeric-range проблемы; BIGINT identity компактнее и тоже допустим для одной БД. UUID не авторизация и не barcode. TIMESTAMPTZ/UTC RFC3339; occurred_at — server учётное время, не physical/COMMIT time; порядок одной АКБ version.

## Человекочитаемый REST API v1

Префикс /api/v1. R — читать, M — вести employees/cards, C — действие АКБ; это capabilities для согласования, не утверждённые должности. Все маршруты trusted access. Изменяющие POST/PATCH требуют ключ; GET/read-only resolution без ключа. Общие 400/401/403/422/500/503, 404 для отсутствующей сущности/credential. 404 места/disabled place отсутствуют: места не ведутся.

| METHOD PATH | Вход / назначение | Ответ | Ошибки предмета | Повтор / право |
|---|---|---|---|---|
| GET /employees | is_active, cursor/limit | 200 EmployeePage | 400 cursor | read/R |
| POST /employees | display_name, optional personnel_number | 201 Employee | 409 PERSONNEL_NUMBER_EXISTS | key/M |
| GET /employees/{employee_id} | Сотрудник | 200 Employee | 404 | read/R |
| PATCH /employees/{employee_id} | display_name и/или is_active | 200 Employee | 409 EMPLOYEE_HAS_CUSTODY | key/M |
| POST /credential-resolutions | credential_value | 200 CredentialResolution | 404 CREDENTIAL_NOT_FOUND; 403 inactive | read/R |
| GET /employees/{employee_id}/credentials | Активные/исторические карты | 200 CredentialPage | 404 | read/M |
| POST /employees/{employee_id}/credentials | value, optional replaces_credential_id | 201 Credential | 409 ACTIVE_CREDENTIAL_EXISTS/CREDENTIAL_REUSE_UNCONFIRMED | key/M |
| PATCH /employees/{employee_id}/credentials/{credential_id} | is_active=false | 200 Credential | 404 | key/M |
| GET /batteries | inventory_code exact, status, holder_employee_id, location exact, cursor/limit | 200 BatteryPage | 422 filters | read/R |
| POST /batteries | inventory_code, actor_credential_value, destination_location, optional serial_number | 201 CommandResult(STORE) | 409 INVENTORY_CODE_EXISTS/LOCATION_OCCUPIED | key/C+регистрация |
| GET /batteries/{battery_id} | Current location/holder/version | 200 Battery | 404 | read/R |
| POST /batteries/{battery_id}/take | actor_credential_value; optional expected_version/observed_source_location | 201 CommandResult | 409 BATTERY_ALREADY_ISSUED/INVALID_BATTERY_STATE/SOURCE_MISMATCH/STATE_VERSION_MISMATCH | key/C |
| POST /batteries/{battery_id}/return | actor_credential_value, destination_location; optional expected_version | 201 CommandResult | 403 RETURN_NOT_ALLOWED; 409 state/version/LOCATION_OCCUPIED | key/C |
| POST /batteries/{battery_id}/move | actor_credential_value, destination_location; optional expected_version/observed_source_location | 201 CommandResult | 409 SAME_LOCATION/state/source/version/LOCATION_OCCUPIED | key/C |
| GET /batteries/{battery_id}/operations | History version DESC | 200 OperationPage | 404 | read/R |
| GET /employees/{employee_id}/batteries | Current custody | 200 BatteryPage | 404 | read/R |
| GET /employees/{employee_id}/operations | actor OR source/destination holder | 200 OperationPage | 404 | read/R |
| GET /operations | battery_id, employee_id, location, device_code, type, from/to, cursor/limit | 200 OperationPage | 422 range | read/R |
| GET /operations/{operation_id} | Операция | 200 Operation | 404 | read/R |

По точному location содержимое возвращает 0/1 АКБ; пустой список не подтверждает существование пустой ячейки. Location в URL query кодируется при специальных символах. Списки {items,next_cursor}; limit default50/max200 техническое предложение. Registry по id, battery history version DESC, общий history occurred_at/id DESC. Cursor связан с фильтрами/upper boundary истории; bad cursor400; from inclusive/to exclusive, from>=to422. Current paging не обещает frozen snapshot.

Пример MOVE body: `{"actor_credential_value":"0004097281923","destination_location":"2.1.2","expected_version":3,"observed_source_location":"1.2.4"}`. Backend сохраняет свой source 1.2.4; observed source только сверяется. Client не устанавливает current/status/holder/device.

## Ограничения, HTTP и Swagger

Нет списка всех пустых мест/числа полок/ячеек, FK существования места, active flag адреса или управления конфигурацией. Addresses из history — только наблюдавшиеся строки. Новые требования такого рода означают пересмотр D09, не молчаливое добавление трёх таблиц.

200 read/PATCH; 201 created resource/event; 400 syntax/UUID/key/cursor/unknown field; 401 no authentication; 403 право/inactive actor/чужой RETURN; 404 battery/employee/credential/event; 409 state/version/source/occupied/key; 422 semantic DTO (включая пустой/крайние пробелы адреса); 500 unexpected; 503 retry/неопределённый результат; 413/415 body size/Content-Type.

`{"error":{"code":"LOCATION_OCCUPIED","message":"Место уже занято","details":{"location":"2.1.2"},"request_id":"trace-example"}}`

Не раскрывать SQL/stack/credential/token; stable error code, message для человека. [openapi.yaml](openapi.yaml) описывает 19 операций, схемы/examples/security/operationId. Будущий server: /openapi.yaml и локальный /swagger/, redirect /swagger, same-origin /api/v1, Authorize и ключ для Try it out. Этот YAML автоматически не переключает runtime старой демонстрации.

## Следующий этап

Уточнить Q02–05/Q19/Q25/Q29 и единое написание адреса Q30, затем согласовать API/миграцию/Go. Если переносить existing data, backfill JOIN cabinets/shelves/cells→канонический text для current и обоих концов history, проверить уникальность/версии/FK и old operation/credential policies. Не изменять applied 001_initial.sql, не делать DROP working data. Schema.sql — для пустой тестовой БД.
