# Pandora — учёт АКБ

## Структура

```text
cmd/pandora/       HTTP-сервер
cmd/migrate/       применение миграций
internal/
  config/          настройки DATABASE_URL и HTTP_ADDR
  database/        подключение к PostgreSQL
  model/           сущности и параметры команд
  handler/         HTTP, JSON и проверка запросов
  service/         правила учёта АКБ
  repository/      SQL и транзакции
migrations/        схема базы данных
swagger/           OpenAPI и Swagger UI
scripts/           локальный запуск и настройка PostgreSQL
```

## Запуск и остановка

```sh
make local
```

```sh
make run HTTP_ADDR=127.0.0.1:18081
make migrate MIGRATION_DATABASE_URL='postgres://owner@localhost/pandora_storage?sslmode=disable'
```

Из корня проекта:

```powershell
./scripts/run-local.ps1
```

Скрипт устанавливает и запускает локальный PostgreSQL, создаёт БД `pandora_storage`, применяет миграции и запускает сервер. Swagger с кнопкой **Try it out**: http://127.0.0.1:18080/swagger/.

Для остановки PostgreSQL:

```powershell
./.local/pgsql/bin/pg_ctl.exe -D .local/pgdata -m fast -w stop
```

При ручном запуске задать `DATABASE_URL`; необязательный `HTTP_ADDR` задаёт адрес HTTP-сервера. Файл `.env` автоматически не читается.

```powershell
$env:DATABASE_URL='postgres://pandora_owner@127.0.0.1:55432/pandora_storage?sslmode=disable'
go run ./cmd/migrate
./scripts/postgres-grant.ps1
$env:DATABASE_URL='postgres://pandora_app@127.0.0.1:55432/pandora_storage?sslmode=disable'
$env:HTTP_ADDR='127.0.0.1:18080'
go run ./cmd/pandora
```

Вместо локального PostgreSQL можно запустить `docker compose up -d`.

```powershell
docker compose exec -T -u postgres postgres psql -U pandora_owner -d pandora_storage -v ON_ERROR_STOP=1 -f /opt/pandora/postgres-grants.sql
```

Оба варианта используют порт `55432`.

## Работа с АКБ

- STORE — регистрация и размещение АКБ.
- TAKE — выдача сотруднику.
- RETURN — возврат только сотрудником, который взял АКБ.
- MOVE — перемещение на другой адрес.

Адрес хранится строкой `шкаф.полка.ячейка`, например `1.1.1`: три положительных числа без ведущих нулей. В `battery_operations` это `source_location` и `destination_location`, в `batteries.current_location` - текущий адрес.

API под `/api` показывает состояние АКБ, содержимое адреса, выдачи сотруднику и историю.
Сотрудник может взять любую хранящуюся АКБ; предварительного закрепления за ним нет.

Три постоянных ШК: карта сотрудника (`actor_credential_value`), инвентарный код АКБ
(`inventory_code`), адрес ячейки (`destination_location`). Они не меняются при выдаче
или возврате. ШК карты можно заменить отдельной операцией, прежняя карта остаётся в истории.

| Действие | Маршрут POST | Обязательные поля JSON |
|---|---|---|
| Регистрация | `/api/batteries` | `inventory_code`, `actor_credential_value`, `destination_location` |
| Взять | `/api/batteries/take` | `inventory_code`, `actor_credential_value` |
| Вернуть | `/api/batteries/return` | `inventory_code`, `actor_credential_value`, `destination_location` |
| Переместить | `/api/batteries/move` | `inventory_code`, `actor_credential_value`, `destination_location` |

Сотрудник выбирает «Взять» или «Вернуть», затем сканирует карту и саму АКБ.
При возврате сканирует также свободную ячейку. Если адрес настроен на стационарном
терминале, он передаётся в том же поле без отдельного скана. Сервер не привязывает
терминалы к ячейкам и не переключает TAKE/RETURN автоматически. После 201 терминал
показывает успех, и сотрудник завершает физическую выдачу или возврат.

UUID сохраняется в ответах, маршрутах чтения и истории. Старые POST-маршруты
`/api/batteries/{battery_id}/take|return|move` заменены командами из таблицы.

## Пример: путь TAKE

```http
POST /api/batteries/take
Content-Type: application/json

{"actor_credential_value":"CARD-A","inventory_code":"AKB-001"}
```

Возврат той же АКБ:

```http
POST /api/batteries/return
Content-Type: application/json

{"actor_credential_value":"CARD-A","inventory_code":"AKB-001","destination_location":"1.2.3"}
```

1. handler проверяет JSON, обязательные поля, формат адреса и опциональной версии.
2. service начинает транзакцию, проверяет и блокирует сотрудника, карту, затем АКБ
   по буквальному инвентарному коду. TAKE требует STORED; RETURN — ISSUED и текущего держателя.
3. repository меняет состояние, адрес/держателя и версию, добавляет историю.
   Состояние и операция фиксируются одним commit; ответ 201 содержит `battery` и `operation`.

RETURN чужой АКБ даёт 403; занятая ячейка и неверное состояние — 409.
`expected_version` необязателен для TAKE/RETURN/MOVE и позволяет отклонять команды
по устаревшему состоянию. TAKE/MOVE также принимают `observed_source_location`.
Карта определяет участника операции; HTTP API публичный, отдельной аутентификации нет.

## Повтор и потерянный ответ

Ключи идемпотентности и хранение ответов удалены со всех изменяющих запросов.
Сервер не требует `Idempotency-Key`, не возвращает `Idempotency-Replayed` и не
обрабатывает этот ключ, даже если старый клиент продолжает его присылать.
Повтор TAKE уже выданной АКБ даёт 409, даже тому же сотруднику. Повтор RETURN уже
хранящейся АКБ также даёт 409. Версия и история при отказе не изменяются.

Обрыв связи не отменяет уже выполненный commit. Если ответ потерян, сначала прочитайте
`GET /api/batteries?inventory_code=AKB-001`: состояние ISSUED и держатель показывают,
кому АКБ сейчас выдана. Этот запрос не восстанавливает результат потерянной команды.
Если состояние не соответствует ожидаемому, нужна сверка перед новым действием.
Без expected_version отложенный повтор после промежуточных RETURN/TAKE может оказаться
допустимой новой командой; автоматические повторы API не гарантирует.

Обновление существующей установки: остановите старый сервер, примените новую миграцию
004 владельцем БД, повторите `scripts/postgres-grants.sql`, запустите новый сервер.
Старые миграции не менялись. Миграция удаляет кеш ответов и служебные ключи истории,
сохраняя бизнес-поля операций. Старый бинарник с новой схемой несовместим.

## Тестовые данные

При запущенном сервере в другом терминале:

```sh
make seed
# другой порт:
make seed SEED_URL=http://127.0.0.1:18081
```

Без Make: `python scripts/seed-demo.py --url http://127.0.0.1:18080`.
Набор содержит 8 сотрудников, 9 карт, 24 АКБ и 50 операций: хранение, выдачу,
возврат, перемещение, замену карты и отключение сотрудника.

Повтор сверяет существующие ресурсы и историю через GET, пропускает выполненные
переходы и продолжает незавершённые. После потери ответа можно повторить запуск:
перед записью генератор снова прочитает историю. Локальный журнал ключей и --state-file
больше не нужны. Префикс DEMO- зарезервирован для генератора; запускайте один экземпляр
и не меняйте демо-набор одновременно. Если АКБ использовали вне демо-сценария,
генератор сообщит о расхождении и не перезапишет её состояние. Автоматического сброса нет.

## Проверки

```sh
make build             # бинарники bin/pandora и bin/migrate
make check             # форматирование, vet, тесты и сборка
make test-race         # тесты с --race
make db-up
```

Локальный запуск создаёт также отдельную БД `pandora_test` для интеграционных тестов:

```powershell
$env:TEST_DATABASE_URL='postgres://pandora_owner@127.0.0.1:55432/pandora_test?sslmode=disable'
$env:TEST_APP_DATABASE_URL='postgres://pandora_app@127.0.0.1:55432/pandora_test?sslmode=disable'
go test ./... -count=1
go vet ./...
go build ./cmd/pandora
go build ./cmd/migrate
```
