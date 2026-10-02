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

API под `/api` показывает текущее состояние, содержимое адреса, выданные сотруднику АКБ и историю. Изменяющим запросам нужен UUID в заголовке `Idempotency-Key`: повтор с тем же ключом и телом возвращает сохранённый результат.

## Тестовые данные

При запущенном сервере (`make local` или `make run`) в другом терминале:

```sh
make seed
# другой порт:
make seed SEED_URL=http://127.0.0.1:18081
```

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
