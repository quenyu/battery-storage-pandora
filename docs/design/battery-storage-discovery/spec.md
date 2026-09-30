# Спецификация v2: учёт АКБ с текстовыми местами

> Статус реализации 30.09.2026: проверенный локальный MVP создан на ветке feature/text-location-mvp. Подтверждённый пользователем формат адреса — три положительных числа шкаф.полка.ячейка без ведущих нулей (Q30 закрыт). Сотрудники и карты могут быть тестовыми локальными данными. Корпоративный источник и authentication/authorization ещё не согласованы. Текущий запуск и результаты проверок: README.md в корне и verification.md. Ниже сохранено проектное обоснование; прежние пометки «backend ещё не реализован» относятся к состоянию до этого этапа.


Статус: готово для обсуждения API; готово частично для реализации. Руководитель, по сообщению пользователя, согласовал хранение мест текстом в battery_operations без таблиц шкафов/полок/ячеек. Пользователь ранее подтвердил одну АКБ на ячейку и возврат только взявшим. Остальные технические решения предложены. Открытые вопросы — [research.md](research.md).

## MVP и границы

Назначение: для индивидуальной АКБ ответить, где она сейчас или кто держатель, и показать все подтверждённые движения с участником, временем и известной меткой ТСД. Go-монолит/pgx/явный SQL/PostgreSQL/REST/OpenAPI заданы пользователем.

Четыре domain таблицы: employees, employee_credentials, batteries, battery_operations. Одна техническая idempotency_requests сохраняет result повторов. История — source/destination_location TEXT; current_location в batteries для быстрого состояния и уникальной занятости. Отдельной devices нет, nullable device_code в операции. Одна команда — одна АКБ; SELF TAKE actor становится holder.

Входит: сотрудники/несколько credential, регистрация+STORE, TAKE/RETURN/MOVE, поиск/текущее состояние, история АКБ/сотрудника/адреса, содержимое адреса, custody, безопасные повторы и деактивация сотрудников/карт. Не входит: конфигурация/создание/отключение мест, полный список пустых ячеек, FK физического существования адреса, lifecycle устройств, клиент ТСД/offline, locks шкафа, датчики/нормативы, UI, full IAM, batch/TRANSFER. Прочие операции/статусы после подтверждения, не молчаливое расширение.

## Функциональные требования и приёмка

| ID | Требование | Наблюдаемая приёмка |
|---|---|---|
| R01 | Employee ID, personnel number и credential value отделены | Смена карты сохраняет employee/history; ведущие нули остаются |
| R02 | По решению руководителя адреса TEXT без таблиц мест | MOVE хранит оба адреса; current_location совпадает с последней destination |
| R03 | Индивидуальный реестр и UNIQUE inventory_code | Второй код409, формат маркировки не выдуман |
| R04 | Immutable journal плюс быстрая projection | GET current читает batteries; история не переписывается |
| R05 | STORE/TAKE/RETURN/MOVE — явные действия | Произвольного PATCH status/holder/location нет |
| R06 | State/event/result атомарны | Inject failure даёт полный rollback |
| R07 | Повторы с key без нового эффекта | Повтор возвращает исходный response даже после дальнейшего RETURN |
| R08 | Конкурентные TAKE/занятие адреса корректны | Один успешный TAKE; две АКБ не занимают одинаковый текст |
| R09 | Actor, old/new holder, source/dest, timestamp/device trace | RETURN actor=holder; MOVE сохраняет оба конца |
| R10 | Исторические сущности не удаляются, snapshots не меняются | Смена карты сохраняет прежний value; MOVE не меняет прежние адреса |
| R11 | Один API ТСД/Web и OpenAPI | DTO/errors/operationId согласованы с таблицей архитектуры |
| R12 | Идентификация отделена от доступа | Card определяет actor; trusted context определяет permissions |
| R13 | Source — server truth | Observed source только сверяется с locked current |
| R14 | Среда и измерения АКБ разделены | Не добавлены temperature/humidity в batteries; future отдельные time series |

## Инварианты

1. ID человека устойчив. Card replacement отключает только выбранную старую; несколько active cards допустимы. Active value уникален, value/employee_id immutable. Повторное использование исторического значения — Q26.
2. STORED = location NOT NULL / holder NULL; ISSUED = location NULL / holder NOT NULL. Никаких других финальных статусов без предметного подтверждения. Атомарная регистрация+STORE/v1 исключает unplaced состояние; если нужно — Q29.
3. TAKE только STORED→ISSUED, actor=новый holder. RETURN только ISSUED→STORED и actor=заблокированный current holder. Сравнивается employee.id, а не карта; чужой возврат403 RETURN_NOT_ALLOWED. MOVE только STORED→STORED с разными адресами.
4. Одна АКБ на точный канонический location. В DDL partial UNIQUE текущего текста. Предлагаем непустой текст без крайних обычных пробелов; формат/aliases уточнить Q30. Разные написания физически одного места могут обойти unique, пока не согласовано единое представление.
5. Каждой version соответствует одна operation. Source совпадает с концом предыдущей версии, latest event/current/version/time совпадают при COMMIT. Version растёт на1, порядок истории не зависит от wall clock.
6. FK employees/credentials/battery/request — RESTRICT. Event UPDATE/DELETE/TRUNCATE forbidden; runtime не owner/superuser. Текстовые адреса — snapshots, не FK. Переименование/закрытие физического места не редактирует старые snapshots.
7. Нельзя обещать проверку физического существования/disabled места, число полок, полный перечень свободных ячеек: такого источника данных в согласованной модели нет. Пустой exact GET — отсутствие учтённой АКБ, а не подтверждение существующего места.
8. Предлагается не отключать employee с custody без сверки. Card revocation holder не меняет; RETURN возможен новой картой того же сотрудника. Названия/правила ролей и ограничения количества АКБ на человека неизвестны.

## Разделение DB и Go

| Инвариант | PostgreSQL | Go / согласование |
|---|---|---|
| Identity/refs | PK/FK/NOT NULL/UNIQUE | Actor resolution, права, источник данных |
| State XOR holder/location | CHECK | Переходы под FOR UPDATE |
| Одна АКБ на address | Partial UNIQUE current_location | Канонический текст, mapping constraint→409 LOCATION_OCCUPIED |
| Current/history | Version UNIQUE, shape CHECK, deferred chain/projection guard | Общая tx, корректный old source, version+1 |
| RETURN holder | Shape CHECK actor=source holder | Locked current holder comparison→403 |
| Append-only | Triggers и runtime GRANT | Нет HTTP редактирования событий; future correction event |
| Key/result | PK(scope,key), complete/immutable guards | Canonical hash/cache/replay/savepoint |

Физическое место, aliases разных strings и его отключение БД не гарантирует. Owner может обойти защиту; immutable log здесь защищает от ошибки обычного приложения, не от владельца кластера.

## Нефункциональные требования

- TIMESTAMPTZ, server timestamps, API UTC RFC3339; client timezone только отображение. Occurred_at — оформление, не доказанное physical time.
- READ COMMITTED/FOR UPDATE для battery; UNIQUE для конкурентного destination. Success после COMMIT; COMMIT/network uncertainty → тот же key, не новая команда.
- Parameterized pgx SQL, structured errors; сырой PG/SQL/stack/token/credential наружу не отдавать.
- Trusted server authorization всех routes. Bearer interface предложен, provider/issuer/corporate conventions Q19/Q25 неизвестны. Карта не заменяет authentication.
- Structured logs request_id/operation_id/code/duration/replay; не писать raw credentials/token. Метрики lock waits/conflicts/replay. Нагрузку/SLO/timeouts/RPO/RTO/retention получить у предприятия, цифры не выдуманы.
- Backup/restore перед эксплуатацией; current/history/key результаты восстанавливаются вместе. Key результаты пока без автоочистки; retry horizon определить до retention.
- Cursor paging и indexes типовых reads. Будущий Swagger локальные assets, единый выбранный YAML, Try it out; сейчас создан контракт, не новый server.

## Future scope

INSPECT/MARK_DAMAGED/QUARANTINE/RELEASE/WRITE_OFF/LOST/FOUND требуют реальных переходов и согласованных migrations. QUARANTINE может быть характеристикой доступности при существующем месте, не автоматически новым видом location. Battery_types/roles/permissions/audit_log — только по подтверждённым нуждам. Admin audit полезен до эксплуатации, но не смешивается с движением АКБ.

Environment readings: отдельный ряд с storage_location TEXT/sensor, temperature/humidity, measured_at/received_at после определения идентичности места/датчика. Battery_measurements: battery_id, voltage/SOC/SOH, measured_at/received_at. Единицы/нормативы/частоту не придумывать. Если нужна полная конфигурация/отключение мест, это конкретный повод пересмотреть D09; скрыто добавлять registry вместо решения руководителя нельзя.

## Продолжение

Достаточно для следующего проектного шага API на новой модели. Реализация требует подтверждения идентификаторов, кадрового источника, доступа и отдельной регистрации, особенно канонизации адреса Q30. Current projection, SQL и OpenAPI технически предложены; schema не production migration. Старые схемы/проверки с FK мест считаются заменёнными. Backend не реализуется в текущем этапе.
