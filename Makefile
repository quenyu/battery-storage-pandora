.DEFAULT_GOAL := help

GO ?= go
GOFMT ?= gofmt
DOCKER ?= docker
PYTHON ?= python3
SEED_URL ?= http://127.0.0.1:18080

DATABASE_URL ?= postgres://pandora_app@127.0.0.1:55432/pandora_storage?sslmode=disable
MIGRATION_DATABASE_URL ?= postgres://pandora_owner@127.0.0.1:55432/pandora_storage?sslmode=disable
HTTP_ADDR ?= 127.0.0.1:18080
TEST_DATABASE_URL ?= postgres://pandora_owner@127.0.0.1:55432/pandora_test?sslmode=disable
TEST_APP_DATABASE_URL ?= postgres://pandora_app@127.0.0.1:55432/pandora_test?sslmode=disable

.PHONY: help build run migrate local seed fmt fmt-check vet check db-up db-down db-grants clean

help:
	@printf '%s\n' \
	  'build             бинарник/' \
	  'run               http' \
	  'migrate           миграции' \
	  'local             запустить, миграции, права' \
	  'seed              загрузить демо-данные через запущенный API' \
	  'fmt / fmt-check   форматирование' \
	  'vet               go vet' \
	  'check             форматирование, билд' \
	  'db-up / db-down   db через docker' \
	  'db-grants         права' \
	  'clean             удалить бинарники'

build:
	mkdir -p bin
	$(GO) build -o bin/pandora ./cmd/pandora
	$(GO) build -o bin/migrate ./cmd/migrate

run:
	DATABASE_URL="$(DATABASE_URL)" HTTP_ADDR="$(HTTP_ADDR)" $(GO) run ./cmd/pandora

migrate:
	DATABASE_URL="$(MIGRATION_DATABASE_URL)" $(GO) run ./cmd/migrate

local:
	$(MAKE) db-up
	$(MAKE) migrate
	$(MAKE) db-grants
	$(MAKE) run

seed:
	$(PYTHON) scripts/seed-demo.py --url "$(SEED_URL)"

fmt:
	$(GOFMT) -w cmd internal migrations swagger

fmt-check:
	@files=$$($(GOFMT) -l cmd internal migrations swagger) || exit $$?; \
	if [ -n "$$files" ]; then printf 'Запустите make fmt:\n%s\n' "$$files"; exit 1; fi

vet:
	$(GO) vet ./...

check: fmt-check vet test build

db-up:
	$(DOCKER) compose up -d --wait postgres

db-down:
	$(DOCKER) compose down

db-grants:
	$(DOCKER) compose exec -T -u postgres postgres psql -U pandora_owner -d pandora_storage \
	  -v ON_ERROR_STOP=1 -f /opt/pandora/postgres-grants.sql

clean:
	rm -f bin/pandora bin/migrate coverage.out
