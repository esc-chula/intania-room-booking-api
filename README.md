# Chula Room Booking API

Go backend foundation using Chi v5, pgx v5/pgxpool, PostgreSQL, log/slog,
Swaggo v1, and Goose SQL migrations. The first migration defines users,
buildings, rooms, bookings, room scheduling, and audit logs. There are no
business APIs, SSO/session implementations, or equipment features yet.

```text
HTTP handler -> Domain service -> Domain port
                                     ^
                                     |
                            PostgreSQL adapter

cmd/api wires config -> logger -> pgxpool -> health handler -> Chi -> HTTP server
```

Domain folders are placeholders. Operational health handlers need no domain
service. Future repositories use parameterized pgx SQL directly, without an ORM
or sqlc. The temporary Go module path is `example.com/chula-room-booking` because
this repository has no Git remote; replace it and its imports when the real
module path is known.

## Local development

Prerequisites: Go 1.26.8, Make, a POSIX shell, Docker with Compose, and curl for
smoke checks. The Makefile uses standard gofmt, mktemp, and diff utilities.

```sh
cp .env.example .env
make db-up
make run
```

`make run` sources the local `.env` as shell assignments and exports them.
When running `go run ./cmd/api` directly, export environment variables yourself.
The application does not read `.env` or fall back to `.env.example`.
`DATABASE_URL` is required. Other settings default to the values in `.env.example`.
Supported `APP_ENV` values are `development` and `production`; log levels are
`debug`, `info`, `warn`, and `error`. Pool maximum must be positive, minimum
between zero and maximum, and both timeouts positive. HTTP_ADDR is host:port.

Compose runs PostgreSQL 18 only, binds localhost:5432, and retains data in a
named volume. `chula`/`localdev` and `sslmode=disable` are for local development
only. Supply production credentials and appropriate PostgreSQL TLS settings at
runtime; the driver respects the DSN. `make db-down` stops Compose and preserves
the volume. No migrations run during startup.

## Probes and Swagger

- `GET /healthz`: 200 `{"status":"ok"}`, independent of PostgreSQL after startup.
- `GET /readyz`: a two-second PostgreSQL ping; 200 `{"status":"ready"}` or
  503 `{"status":"unavailable"}`. Both probes return JSON with `Cache-Control: no-store`.
- Development docs: http://localhost:8080/swagger/index.html and
  http://localhost:8080/swagger/doc.json. Both return 404 in production.
- All unknown routes, including `/api/v1/rooms`, return 404. Future business
  routes will use `/api/v1`. Swagger documents the two root-level probes; its
  base path is `/` so their URLs are accurate. Future business route annotations
  must include `/api/v1` in their paths. This changes the Swagger base path
  specified in the Phase 0 initialization plan.

The process fails startup if PostgreSQL is unavailable. SIGINT/SIGTERM drain
HTTP requests within SHUTDOWN_TIMEOUT (10 seconds by default), then close the
pool. HTTP header/read/write/idle timeouts are 5/10/15/60 seconds. Request logs
include method, route/path, status, latency, and request ID, without query
strings, cookies, authentication headers, or bodies. Proxy headers are not used
to determine client IP.

```sh
make tools          # local .bin/swag, pinned v1.16.6
make swagger        # regenerate committed Go/JSON/YAML docs
make check          # formatting check, vet, unit tests, build, docs drift
go build ./...      # all packages
```

Individual targets: `make fmt`, `make vet`, `make test`, `make build`, and
`make swagger-check`. Unit checks require no PostgreSQL. The drift check compares
a freshly generated temporary directory against docs, including added/missing
files; it works before the first Git commit as well as in CI. Keep generated
docs in version control so ordinary builds need no generator. Handler tests and
review must still check that annotations match actual behavior.

Smoke checks while `make run` is running:

```sh
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/readyz
curl -i http://localhost:8080/swagger/doc.json
curl -i http://localhost:8080/api/v1/rooms
docker compose stop db
curl -i http://localhost:8080/healthz  # still 200
curl -i http://localhost:8080/readyz   # now 503
make db-up
```

Stop the API with Ctrl-C or SIGTERM. GitHub Actions runs checks without a DB.

## Container and migrations

```sh
docker build -t chula-room-booking-api .
docker run --rm -p 127.0.0.1:8080:8080 \
  -e DATABASE_URL='postgres://USER:PASSWORD@DB_HOST:5432/DB_NAME?sslmode=verify-full' \
  chula-room-booking-api
```

The runtime is non-root with CA certificates; APP_ENV defaults to production.
Supply a DB host reachable from the container (localhost points to the container).
No environment files or credentials are copied into the image.

Install the pinned Goose v3 CLI, then migrate the database separately from API
startup:

```sh
go install github.com/pressly/goose/v3/cmd/goose@v3.24.3
export DATABASE_URL='postgres://chula:localdev@localhost:5432/chula_room_booking?sslmode=disable'
goose -dir db/migrations postgres "$DATABASE_URL" up
# To roll back the latest migration:
goose -dir db/migrations postgres "$DATABASE_URL" down
```

Future authentication uses Chula SSO and PostgreSQL-backed server-side sessions,
not JWT. Secure session cookies and CSRF protection belong to that future phase;
Swagger 2.0 cannot natively model cookie authentication. This first migration
extends the preserved Phase 0 initialization plan using the DBML supplied for
this change; it excludes equipment and external identities. The team's API v0.2,
DBML v1, and EPIC remain future functional sources of truth and are not present
in this repository.
