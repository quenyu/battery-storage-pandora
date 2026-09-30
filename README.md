# Pandora — Go backend учёта АКБ

Проверяемый локальный MVP: Go, PostgreSQL, pgx через database/sql, явный SQL и REST. Актуальный контракт встроен из `docs/design/battery-storage-discovery/openapi.yaml`. Локальные assets Swagger UI включены в бинарник; доступ в интернет для Try it out не нужен.

Сотрудники и несколько карт — отдельные сущности. Регистрация АКБ создаёт STORE/v1; далее TAKE, RETURN только держателем и MOVE. Адрес — текст `шкаф.полка.ячейка`: три положительных числа без ведущих нулей. Число шкафов/полок/ячеек не ограничивается выдуманными значениями. Адреса в журнале — снимки; current_location — проекция. Частичный UNIQUE допускает одну АКБ на адрес.

## Быстрый запуск Windows

Нужны Go >=1.24 и Git. Portable PostgreSQL уже может быть в `.local/pgsql`; скрипт установки при отсутствии скачивает закреплённый официальный архив. Docker — альтернативный вариант ниже.

```powershell
# Из корня репозитория; новая отдельная demo БД, старую pandora не мигрирует.
./scripts/run-local.ps1 -HttpAddress 127.0.0.1:18080
```

Откройте http://127.0.0.1:18080/swagger/. Нажмите Authorize и введите `local-mvp-test-token` (без слова Bearer). Каждая изменяющая команда требует нового UUID Idempotency-Key; при timeout повторяйте тот же ключ и тело. `POST /credential-resolutions` — чтение без ключа.

В другом PowerShell можно создать тестового сотрудника, карты и АКБ и проверить STORE→TAKE→смена карты→RETURN→MOVE→replay:

```powershell
./scripts/demo.ps1 -Base http://127.0.0.1:18080/api/v1 -Token local-mvp-test-token
```

Скрипт добавляет данные, не очищает БД. Коды/адреса каждого запуска отдельные. Ctrl+C останавливает API; PostgreSQL можно остановить `./scripts/postgres-stop.ps1`.

## Ручной запуск / Docker

`.env.example` описывает переменные; приложение само файл .env не читает. `DEV_MODE=true` и явно заданный `DEV_API_TOKEN` включают локальный адаптер доступа. Карта сотрудника определяет actor, token — API principal; значение token не записывается в БД. При смене token сохраняется scope `local-mvp`, поэтому replay остаётся доступным. Без настроенного адаптера запуск сервера отклоняется. Корпоративный issuer, реальные роли и кадровый источник не придуманы.

```powershell
docker compose up -d
$env:DATABASE_URL='postgres://pandora_owner@127.0.0.1:55432/pandora_mvp_demo?sslmode=disable'
go run ./cmd/pandora migrate
docker compose exec -T -u postgres postgres psql -U pandora_owner -d pandora_mvp_demo -v ON_ERROR_STOP=1 -f /opt/pandora/postgres-grants.sql
$env:DATABASE_URL='postgres://pandora_app@127.0.0.1:55432/pandora_mvp_demo?sslmode=disable'
$env:DEV_MODE='true'
$env:DEV_API_TOKEN='local-mvp-test-token'
$env:HTTP_ADDR='127.0.0.1:18080'
go run ./cmd/pandora serve
```

При существующем Docker volume init script автоматически повторно не выполняется: выполните `postgres-init.sql` отдельно или создайте demo/test БД с указанными owner/grants. Не удаляйте volume с данными. Docker и portable cluster используют один порт — выбирайте один вариант. Предоставленный кластер с trust authentication предназначен для локальных тестов на loopback.

## Перенос старой демонстрации

`001_initial.sql` сохранён без изменений. `002_text_locations.sql` выполняется атомарно вместе с записью checksum и под migration lock. Старый сервер надо остановить перед согласованным переходом.

Все восемь старых таблиц и их строки сохраняются как неизменяемые `legacy_*` в той же схеме. Новый runtime не читает таблицы мест; ему доступны только пять новых таблиц. Сотрудники/карты/АКБ/операции переносятся с прежними ID, временем и версиями. Адреса JOIN cabinets→shelves→cells равны прежнему API `cabinet.number.shelf.number.cell.number`; это проверено на отдельной тестовой БД. Runtime получает SELECT/INSERT истории, без UPDATE/DELETE/TRUNCATE и без доступа к архиву.

Если старые данные содержат LOSS/LOST, чужой RETURN, разрыв истории или несовпадение текущей проекции, миграция полностью откатывается и сообщает причину. Эти события нельзя молча переписать под новые правила. Требуется отдельное бизнес-решение о переносе несовместимых данных. Нужна резервная копия и проверка на копии БД перед рабочим переходом; рабочая `pandora` в этой разработке не менялась.

Старые глобальные строковые idempotency keys и JSON остаются в `legacy_idempotency_records`. Новый API имеет другие пути/DTO и scope+UUID; старые ответы не выдаются за новый контракт. Для импортированных событий создаются служебные записи scope `legacy-migration`, которые не поддерживают клиентский replay старого API.

## API и транзакции

19 операций под `/api/v1`, описанных в Swagger:

- сотрудники, изменение имени/активности, несколько credentials, замена конкретной карты, отключение и resolution;
- АКБ: регистрация+STORE, TAKE/RETURN/MOVE, текущая проекция, точный поиск по коду, статусу, holder и location;
- custody сотрудника; история АКБ по version DESC; история сотрудника по actor/обоим holders; общий журнал по обоим адресам, устройству, типу и интервалу времени;
- списки `{items,next_cursor}`: limit 50 по умолчанию, максимум 200; keyset cursor связан с endpoint и фильтрами. История имеет верхнюю границу; текущие списки не обещают frozen snapshot.

Одна транзакция: reservation(scope,key) → SAVEPOINT → employee/credential FOR SHARE → battery FOR UPDATE → current/event → первоначальный HTTP result → COMMIT. UNIQUE адреса разрешает гонки разных АКБ. Конфликты 404/409/422, полученные внутри бизнес-транзакции, сохраняются после rollback к savepoint; malformed/auth/500/503 не сохраняются. Ошибки валидации до BEGIN не резервируют ключ. Replay возвращает исходный status/body даже после следующих движений. При переиспользовании ключа для другого метода/пути/тела — 409 IDEMPOTENCY_KEY_REUSED.

JSON-порядок и регистр UUID не меняют hash. Идентификаторы строковые, leading zero карты/кода сохраняются. Source берётся из БД, observed_source только сверяется; expected_version необязателен. Ошибки имеют единый envelope `error {code,message,details,request_id}`; SQL и credentials не раскрываются. X-Request-ID отмечает конкретный HTTP вызов, cached body сохраняет первоначальный request_id. Логи содержат route/status/duration/replay, без тела, query и token.

DB guards запрещают изменение истории и identity, неполный результат и COMMIT с нарушением цепочки/projection. Владелец БД/суперпользователь остаётся границей доверия. Таймауты lock=5s, statement=10s, request=15s — технические настройки локального MVP, не заявленные SLO. Deadlock/timeout возвращает503; клиент повторяет прежний ключ.

## Проверки

```powershell
$env:TEST_DATABASE_URL='postgres://pandora_owner@127.0.0.1:55432/pandora_test?sslmode=disable'
$env:TEST_APP_DATABASE_URL='postgres://pandora_app@127.0.0.1:55432/pandora_test?sslmode=disable'
go test ./... -count=1
go vet ./...
go build ./cmd/pandora
```

Тесты требуют БД с именем `_test`, создают отдельные случайные схемы и удаляют только свои схемы. Без TEST_DATABASE_URL PostgreSQL-тесты явно skipped. Тестовый runtime — ограниченная роль; owner применяется только для setup и fault injection.

Проверяются все19 успешных ответов против OpenAPI и187 examples; два TAKE (один успех), две АКБ на адрес (один успех), конкурирующие MOVE, чужой RETURN, повтор после обрыва TCP после COMMIT, concurrent key, replay после RETURN и saved409, rollback после записи event/current перед result, смена карты и custody, отзыв карты/сотрудника, подмена source/version, диапазоны и cursors, immutable guards и runtime grants, перенос истории/адресов и отказ несовместимых данных. Проверка Go race требует CGO и C-компилятора; в предоставленной Windows-среде они отсутствуют.

Опциональная автоматическая проверка браузера использует уже установленный Playwright:

```powershell
$env:BASE_URL='http://127.0.0.1:18080'
$env:DEV_API_TOKEN='local-mvp-test-token'
node scripts/browser-smoke.cjs
```

Скрипт проверяет локальный ресурсный набор, Authorize,19 карточек,20 schemas, Try it out,201/replay/чтение и STORE; доказательства в `.local/browser-evidence`. Не требуется frontend build.

## Осталось согласовать

Кадровый источник, реальные маркировки и corporate authentication/authorization, повторное назначение исторической карты другому человеку, отдельная регистрация без STORE, retention и backup/restore/RPO/RTO. Пока историческую карту нельзя назначать другому сотруднику; сотрудник с custody не отключается. Это ограниченные правила MVP из design package.

ТСД приложение, оборудование/offline, физическое существование/закрытие адреса, sensors/замки, LOSS/TRANSFER и производственная эксплуатация вне этого MVP.
