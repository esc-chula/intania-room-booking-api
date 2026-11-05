SWAG_VERSION := v1.16.6
GOOSE_VERSION := v3.24.3
TOOLS_DIR := $(CURDIR)/.bin
SWAG := $(TOOLS_DIR)/swag
GOOSE := $(TOOLS_DIR)/goose
SWAG_ARGS := -g main.go -d ./cmd/api,./internal/adapter/primary/http,./internal/core/domain/auth,./internal/core/domain/user,./internal/core/domain/sheetbooking --parseInternal

.PHONY: db-up db-down migrate-up migrate-down bootstrap-admin bootstrap-os run fmt fmt-check vet test build tools swagger swagger-check check

db-up:
	docker compose up -d --wait db

db-down:
	docker compose down

migrate-up:
	@test -x "$(GOOSE)" || { echo 'Run make tools first.' >&2; exit 1; }
	@set -eu; if [ -z "$${DATABASE_URL:-}" ] && [ -f .env ]; then set -a; . ./.env; set +a; fi; \
		: "$${DATABASE_URL:?Set DATABASE_URL or create .env from .env.example}"; \
		"$(GOOSE)" -dir db/migrations postgres "$$DATABASE_URL" up

migrate-down:
	@test -x "$(GOOSE)" || { echo 'Run make tools first.' >&2; exit 1; }
	@set -eu; if [ -z "$${DATABASE_URL:-}" ] && [ -f .env ]; then set -a; . ./.env; set +a; fi; \
		: "$${DATABASE_URL:?Set DATABASE_URL or create .env from .env.example}"; \
		"$(GOOSE)" -dir db/migrations postgres "$$DATABASE_URL" down

bootstrap-admin:
	@test -f .env || { echo 'Create .env from .env.example first.' >&2; exit 1; }
	@set -eu; set -a; . ./.env; set +a; go run ./cmd/bootstrap-admin

bootstrap-os:
	@test -f .env || { echo 'Create .env from .env.example first.' >&2; exit 1; }
	@set -eu; set -a; . ./.env; set +a; go run ./cmd/bootstrap-os

run:
	@test -f .env || { echo 'Create .env from .env.example first.' >&2; exit 1; }
	@set -eu; set -a; . ./.env; set +a; go run ./cmd/api

fmt:
	gofmt -w cmd internal docs

fmt-check:
	@set -eu; files=$$(gofmt -l cmd internal docs); test -z "$$files" || { echo "$$files"; exit 1; }

vet:
	go vet ./...

test:
	go test ./...

build:
	go build ./cmd/api

tools:
	GOBIN="$(TOOLS_DIR)" go install github.com/swaggo/swag/cmd/swag@$(SWAG_VERSION)
	GOBIN="$(TOOLS_DIR)" go install github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)

swagger:
	@test -x "$(SWAG)" || { echo 'Run make tools first.' >&2; exit 1; }
	@test "$$(go version -m "$(SWAG)" | awk '$$1 == "mod" && $$2 == "github.com/swaggo/swag" { print $$3 }')" = '$(SWAG_VERSION)' || { echo 'Run make tools to install Swaggo v1.16.6.' >&2; exit 1; }
	"$(SWAG)" init $(SWAG_ARGS) -o ./docs

swagger-check:
	@test -x "$(SWAG)" || { echo 'Run make tools first.' >&2; exit 1; }
	@test "$$(go version -m "$(SWAG)" | awk '$$1 == "mod" && $$2 == "github.com/swaggo/swag" { print $$3 }')" = '$(SWAG_VERSION)' || { echo 'Run make tools to install Swaggo v1.16.6.' >&2; exit 1; }
	@set -eu; temp=$$(mktemp -d); trap 'rm -rf "$$temp"' EXIT HUP INT TERM; \
		"$(SWAG)" init $(SWAG_ARGS) -o "$$temp/docs"; \
		diff -ru docs "$$temp/docs"

check: fmt-check vet test build swagger-check
