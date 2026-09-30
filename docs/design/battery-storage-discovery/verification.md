# Проверка проектирования v2

Обновлено 30.09.2026 после решения руководителя о текстовых адресах без таблиц мест. Готово для обсуждения API; готово частично для реализации: Q02–05/Q19/Q25/Q29 и канонизация Q30 требуют ответов. Новый backend не реализован, HTTP проверки ниже запланированы. Существующая демонстрация имеет другой контракт и не подтверждает эту версию.

## Выполненные проверки актуальной версии

- OpenAPI 3.0.3 загружен kin-openapi v0.133.0 из текущего go.mod: **19 операций, 20 schemas, 187 request/response examples**; operationId уникальны, ссылки и примеры валидны. В контракте нет cabinet/shelf/cell/device CRUD, cell UUID и location_code projection.
- Текущий schema.sql применён в чистую изолированную PostgreSQL 17.11, database design_text_locations, loopback55433. Старые рабочие БД/миграции не затрагивались; после проверок отдельный кластер остановлен.
- **35 SQL checks/assertions**, включая подготовительные: introspection подтверждает ровно пять таблиц — четыре domain плюс requests, без мест/devices; STORE, TAKE, RETURN, MOVE/current guards; active credential и inventory uniqueness; location XOR holder; incomplete key COMMIT forbidden; immutable result/events/credential value; composite actor/credential FK; запрет чужого RETURN; wrong source/MOVE self rejected; полный rollback; карта/история сохраняются; пустой адрес и крайние пробелы rejected.
- Два независимых SQL-соединения: конкурентные TAKE одной АКБ дают одну выдачу и одного holder; второй после ожидания battery FOR UPDATE видит ISSUED.
- Два независимых SQL-соединения: MOVE разных АКБ на один текстовый адрес без destination row/lock. Первая транзакция удерживает запись уникального адреса до COMMIT, вторая ждёт UNIQUE проверки и получает 23505 именно batteries_one_per_location_uq. Проигравшая сохраняет old location/version, её event/result откатываются.
- Два независимых соединения: одинаковый key reservation ждёт первый COMMIT, отдельный SELECT получает первоначальный result. Это SQL-основа replay, не ещё отсутствующая HTTP реализация.
- Проверен конкретный historical MOVE source=1.2.4,destination=2.1.2; смена credential не меняет прежнее значение в событиях.

Диагностика локально: `.local/design-validation/validate_text_schema.py`, `results-text-locations.json`, `validate.go`, `build_openapi_text_locations.py`. Они не подключены к runtime/миграциям. Прежние 39 проверок с FK/active мест и 355 OpenAPI examples относятся к заменённой версии и не выдаются за текущие. Техническая SQL validation не доказывает корректность ещё ненаписанного Go.

## Сценарии A–H

Фикстура: B-00412 STORED на адресе 1.1.1, version1 после STORE; Петров/Сидоров — разные employee ID с active credentials. Адреса/коды иллюстративны, не реальные данные предприятия. Сценарий C выполняется на отдельной STORED фикстуре, иначе после A батарея уже ISSUED.

| Сценарий | Действие/API | Результат current/history | Защита |
|---|---|---|---|
| A. Петров взял из 1.1.1 | POST /batteries/{id}/take, keyA, credential, expected_version1 | ISSUED, holderПетров, locationNULL,v2; TAKE source_location1.1.1 | Battery FOR UPDATE, source server, atomic event/current/result |
| B. Петров взял повторно | Новый keyB | 409 BATTERY_ALREADY_ISSUED, нет нового события | TAKE требует STORED; тот же keyA — replay успеха, не второе действие |
| C. Сидоров берёт одновременно | Разные keys/connections из STORED | Один201, другой409; одна TAKE | Shared battery row lock; после ожидания актуальное состояние |
| D. Петров вернул в 1.2.4 | POST /return, destination_location1.2.4 | STORED1.2.4,v3,holderNULL; RETURN actor/source holderПетров | Actor=current holder, destination text UNIQUE |
| D2. Сидоров вернул АКБ Петрова | POST /return credentialСидорова | 403 RETURN_NOT_ALLOWED | Сравнение employee ID; shape CHECK actor=source holder |
| E. Перенос 1.2.4→2.1.2 | POST /move, destination_location2.1.2 | STORED2.1.2,v4; оба text-конца в истории | Battery lock, unique current text; failed update rollback сохраняет source |
| F. Timeout TAKE и повтор | Тот же keyA/route/body/scope | Исходный201, новых событий нет | Key/result/event atomic; если COMMIT не было, reservation тоже нет. После D/E replay старый, GET current свежий |
| G. Карта Петрова заменена | Создать credential с replaces_credential_id | Employee прежний, old credential disabled, история содержит old value | FK RESTRICT и identity immutable; RETURN новой картой того же человека разрешён |
| H. Физическое место закрыли | Внешнее действие, не PATCH /cells | Прежние snapshots адреса в истории остаются | **Backend не ведёт отключение места и не может запретить новый STORE по active flag.** Это явно исключено новой моделью; если нужен запрет, пересмотреть D09 |

## Запланированные интеграционные/HTTP проверки

| ID / требования | Действие | Ожидаемое внешнее поведение |
|---|---|---|
| V01 R01 | Неизвестный credential lookup | 404 CREDENTIAL_NOT_FOUND; нет auto-create employee |
| V02 R01/R09 | Смена карты при ISSUED, RETURN новым credential того же человека | 201; old card в TAKE, тот же employee ID |
| V03 R01 | Несколько карт человека / чужое active value | Свои несколько допустимы, conflict409 |
| V04 R02/R08 | Два оформления на одинаковый канонический text | Один201, другой409 LOCATION_OCCUPIED |
| V05 R03/R06 | Параллельная регистрация inventory_code | Один201, другой409 INVENTORY_CODE_EXISTS, нет extra STORE |
| V06 R04/R09 | STORE→TAKE→RETURN→MOVE | Current/version соответствуют четырём событиям и обоим адресам |
| V07 R08 | Два независимых concurrent TAKE | Один201, другой409, один holder/TAKE |
| V08 R08 | Concurrent MOVE разных АКБ на один text | Constraint mapping23505→409 LOCATION_OCCUPIED; loser source/version/event unchanged |
| V09 R07 | Одинаковые concurrent key/body | Initial status/семантически тот же JSON, один эффект |
| V10 R07 | Key с другим route/body/credential | 409 IDEMPOTENCY_KEY_REUSED, ничего не применяется |
| V11 R07 | Потеря response после COMMIT | Повтор исходного key безопасен, нет нового действия |
| V12 R07 | Replay TAKE после later RETURN | Старый response; новый GET показывает current, нет дополнительного TAKE |
| V13 R07 | Сохранённый409, адрес освободили, key повторили | Тот же409; исправленный запрос новый key |
| V14 R06 | Inject failure после event/current/перед result | Полный rollback; key не выглядит successful |
| V15 R06/R07 | COMMIT timeout/обрыв | Same key разрешает unknown outcome; не новая command |
| V16 R05 | TAKE issued, RETURN stored, MOVE issued/same location | Соответствующие409 без нового журнала |
| V17 R09 | Чужой RETURN | 403, current holder сохранён |
| V18 R13 | Observed source/version не совпали | 409 SOURCE_MISMATCH/STATE_VERSION_MISMATCH; server source не подменён |
| V19 R02 | Канонический формат/aliases адреса после ответа Q30 | Подтвердить, что физическое место не представляется двумя разными strings. До этого UNIQUE только точного текста |
| V20 R10 | Отключённый credential/employee | Новая команда forbidden; исторические ссылки доступны по правам |
| V21 R07/R12 | Replay после отзыва карты/после отзыва bearer | Initial result при сохранённых правах principal /401 или403 без утечки |
| V22 R11/R13 | Client прислал current_location/status/holder/device_code в command | 400 unknown field; source/device server контекст |
| V23 R11 | Swagger Try it out с bearer/key для команд | Реальные endpoints соответствуют выбранному YAML; unified errors |
| V24 R11 | ТСД/Web одна команда в допустимых правах | Одинаковая семантика, без клиентских исключений |
| V25 R04 | History по employee/location с обоих концов/device/time | Нет duplicates; from inclusive/to exclusive; cursor filters consistent |
| V26 R11 | Bad JSON/duplicate fields/null/UUID/key; blank address | 400 syntax,422 semantic; PG errors не раскрыты |
| V27 R11 | Body size/Content-Type | 413/415 в согласованных пределах |
| V28 R10 | Runtime DB user меняет event | DB отказ; HTTP UPDATE/DELETE нет |
| V29 NFR | Backup/restore | Восстановлены current/history/keys/credentials; RPO/RTO по согласованным целям |

Swagger /swagger/ и /openapi.yaml будущего сервера используют этот контракт; старый server не переключён. Нет теста «выключить ячейку и блокировать размещение», потому что нет API/registry места. Такую гарантию нельзя доказать простым текстовым snapshot.

## Пределы и следующий шаг

35 SQL checks подтверждают ограничения/протокол на PostgreSQL, не HTTP authorization/hash/error mapping/service retries. Физическое существование, реальная пустота, правильность набранного адреса и aliases невозможно установить одним UNIQUE. Согласовать Q30 на нескольких реальных примерах и переходить к обсуждению API; источник сотрудников/identifiers/permissions/отдельная регистрация всё ещё вопросы. Firmware/UI/offline ТСД не требуются текущим этапом.

## Что я бы показал наставнику перед началом реализации

1. ER: четыре предметные таблицы; employee отдельно от cards; text places без отдельных таблиц, техническая requests отдельно. [er-model.md](er-model.md).
2. STORE/TAKE/RETURN/MOVE, RETURN только holder, MOVE оба конца. [spec.md](spec.md).
3. Current_location в batteries и immutable source/destination text snapshots в журнале, atomic tx и UNIQUE занятости. [architecture.md](architecture.md), [schema.sql](schema.sql).
4. Явные API команды с destination_location, server source, key/replay и LOCATION_OCCUPIED. Таблица [architecture.md](architecture.md), [openapi.yaml](openapi.yaml).
5. Открытые вопросы: канонический адрес Q30, реальные идентификаторы, кадровый источник/доступ и отдельная регистрация. Границы: конфигурация/отключение мест теперь не входят. [research.md](research.md).
