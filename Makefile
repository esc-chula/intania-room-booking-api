SWAG_VERSION := v1.16.6
TOOLS_DIR := $(CURDIR)/.bin
SWAG := $(TOOLS_DIR)/swag
SWAG_ARGS := -g main.go -d ./cmd/api,./internal/adapter/primary/http --parseInternal

.PHONY: db-up db-down run fmt fmt-check vet test build tools swagger swagger-check check

db-up:
	docker compose up -d --wait db

db-down:
	docker compose down

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
