# Chula Room Booking API

Go backend foundation using Chi v5, pgx v5/pgxpool, PostgreSQL, log/slog,
Swaggo v1, and Goose SQL migrations. The first migration defines users,
buildings, rooms, bookings, room scheduling, and audit logs. Email/password
registration and login use stateless JWTs. An explicit command creates the
initial ADMIN account, and an OS/admin-only endpoint reads legacy booking rows
live from Google Sheets. OAuth, native booking APIs, and equipment features are
not implemented yet.

```text
HTTP handler -> Domain service -> Domain port
                                     ^
                                     |
                         PostgreSQL / Google Sheets adapter

cmd/api wires config -> logger -> pgxpool -> repositories/services -> handlers -> Chi
```

Auth and user domain packages implement authentication; other domain folders
remain placeholders. Operational health handlers need no domain service.
Repositories use parameterized pgx SQL directly, without an ORM
or sqlc. The temporary Go module path is `github.com/MadManJJ/room-reservation-api` because
this repository has no Git remote; replace it and its imports when the real
module path is known.

## Local development

Prerequisites: Go 1.26.8, Make, a POSIX shell, Docker with Compose, and curl for
smoke checks. The Makefile uses standard gofmt, mktemp, and diff utilities.

```sh
cp .env.example .env
make db-up
make tools
make migrate-up
make run
```

`make run` sources the local `.env` as shell assignments and exports them.
When running `go run ./cmd/api` directly, export environment variables yourself.
The application does not read `.env` or fall back to `.env.example`.
`DATABASE_URL`, `JWT_SECRET`, `GOOGLE_SPREADSHEET_ID`, and `GOOGLE_SHEET_RANGE`
are required. `BOOTSTRAP_ADMIN_PASSWORD` is required when running
`make bootstrap-admin` against a database without an ADMIN. JWT_SECRET must contain at least
32 bytes excluding surrounding whitespace; use an independently generated
random secret in production (for example, `openssl rand -hex 32`). The example
secret is local-only. Other settings default to the values in `.env.example`.
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
  routes will use `/api/v1`. Swagger documents probes, authentication, and the
  OS spreadsheet endpoint; its
  base path is `/` so their URLs are accurate. Business route annotations
must include `/api/v1` in their paths. This changes the Swagger base path
  specified in the Phase 0 initialization plan.

The process fails startup if PostgreSQL is unavailable. SIGINT/SIGTERM drain
HTTP requests within SHUTDOWN_TIMEOUT (10 seconds by default), then close the
pool. HTTP header/read/write/idle timeouts are 5/10/15/60 seconds. Request logs
include method, route/path, status, latency, and request ID, without query
strings, cookies, authentication headers, or bodies. Proxy headers are not used
to determine client IP.

```sh
make tools          # local .bin/swag and .bin/goose, pinned versions
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
  -e JWT_SECRET="$JWT_SECRET" \
  -e GOOGLE_SPREADSHEET_ID='YOUR_SPREADSHEET_ID' \
  -e GOOGLE_SHEET_RANGE='จองห้อง!A9:AM' \
  -e GOOGLE_APPLICATION_CREDENTIALS=/run/secrets/google-credentials.json \
  -v /absolute/path/to/credentials.json:/run/secrets/google-credentials.json:ro \
  chula-room-booking-api
```

The runtime is non-root with CA certificates; APP_ENV defaults to production.
Supply a DB host reachable from the container (localhost points to the container).
The credential mount above is for a local container; deployed workloads can use
an attached identity instead. No environment files or credentials are copied
into the image.

Install the pinned tools and migrate the database separately from API startup.
The migration targets use `DATABASE_URL` from the environment, or from `.env`
when it is unset:

```sh
make tools
make migrate-up
make migrate-down  # roll back the latest migration
```

The password-auth migration adds a nullable password hash and a unique index
on trimmed, lowercase emails. Existing users are preserved without passwords.
If existing emails collide after normalization, migration fails; resolve those
accounts explicitly before retrying. Rolling back this migration removes all
password hashes but preserves users. The first migration excludes equipment
and external identities. The team's API v0.2,
DBML v1, and EPIC remain future functional sources of truth and are not present
in this repository.

## Authentication

Authentication is available in development and production. This explicitly
replaces the session-based direction in the older Phase 0 plan. Future OAuth
can reuse local users and JWT issuance; no OAuth endpoints are included yet.

| Endpoint | Input | Success |
| --- | --- | --- |
| `POST /api/v1/auth/register` | `name`, `email`, `password` | 201, user and access token |
| `POST /api/v1/auth/login` | `email`, `password` | 200, user and access token |
| `GET /api/v1/auth/me` | `Authorization: Bearer <token>` | 200, current user |

Registration trims names, trims/lowercases email, and requires a nonempty name,
a plain email address, and a password of 8–72 bytes. Password whitespace is
preserved. All new accounts are active `USER_A` users. Clients cannot choose
roles, IDs, or active status. JSON bodies are limited to 16 KiB, with unknown
fields and trailing JSON rejected. Errors use `{"error":"..."}`: 400 for
invalid input, 409 for duplicate registration, 401 for failed authentication,
and 500 for unexpected failures. Unknown, disabled, passwordless, and
wrong-password accounts return the same login error.

Successful registration/login returns
`{"access_token":"...","token_type":"Bearer","expires_in":3600,"user":{"id":"...","name":"...","email":"...","role":"USER_A"}}`.
Passwords use bcrypt at cost 12. JWTs use HS256 with user ID, issue/expiry times,
issuer, and audience; they contain no role or personal data. Authentication
checks the current user in PostgreSQL without storing sessions. Disabled or
deleted accounts are rejected immediately; role changes apply on the next
request. All auth responses use `Cache-Control: no-store`.

```sh
curl -i http://localhost:8080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"name":"Test User","email":"test@example.com","password":"test-password-123"}'

curl -i http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"test@example.com","password":"test-password-123"}'

# Copy access_token from either response.
curl -i http://localhost:8080/api/v1/auth/me \
  -H 'Authorization: Bearer YOUR_ACCESS_TOKEN'
```

In Swagger's Authorize dialog, enter `Bearer YOUR_ACCESS_TOKEN`. Tokens expire
after one hour; log in again afterward. Client logout discards the token;
there is no refresh, logout/revocation, password reset, email verification,
or role-management API. An email/password registration does not establish
university affiliation.

## Bootstrap administrator

`make bootstrap-admin` is an explicit deployment or local-development command.
It connects through `DATABASE_URL`; the database may be on another machine as
long as the command runner has network access and valid credentials. If no
`ADMIN` exists, it creates active `Admin` with email
`admin@gmail.com` and a bcrypt cost-12 hash of
`BOOTSTRAP_ADMIN_PASSWORD`. Set that variable to an 8–72-byte password before
running the command. `BOOTSTRAP_ADMIN_NAME` changes the display name and
defaults to `Admin`.

Once any ADMIN exists, the command does nothing and its account is not updated.
If `admin@gmail.com` is already a non-admin account, the command fails
instead of changing its role or password. Do not commit the bootstrap password.
`ADMIN` is the shared authorization middleware's super-role, so it is accepted
by every role-guarded route.

```sh
make migrate-up
make bootstrap-admin
```

## OS spreadsheet bookings

`GET /api/v1/admin/sheet-bookings` reads the configured Google Sheet on every
request and returns an array of rows with `id`, `name`, `room`, `start_date`,
`end_date`, `start_time`, and `end_time` string fields. It uses columns A, F, N,
P, Q, R, and S from the configured range; short rows produce empty strings.
There is no PostgreSQL import, pagination, or filtering. An empty sheet returns
`[]`. The endpoint is available in development and production; Swagger UI is
development-only.

The endpoint requires `Authorization: Bearer <token>` for a currently active OS
or ADMIN user. Missing or invalid tokens return 401, other roles return 403, and a
Google Sheets fetch failure returns 502. Responses are JSON with
`Cache-Control: no-store`.

Set `GOOGLE_SPREADSHEET_ID` to the spreadsheet ID and `GOOGLE_SHEET_RANGE` to
the range, for example `GOOGLE_SHEET_RANGE='จองห้อง!A9:AM'` in `.env`. Google
Application Default Credentials supply read-only Sheets API access. For local
development, set `GOOGLE_APPLICATION_CREDENTIALS` to an absolute path to a
service-account JSON file outside the repository, or use your local ADC login.
In a deployed environment, attach a suitable identity; when running the
container with a local credential file, mount it and set the variable to its
path inside the container. Grant that identity Viewer access to the sheet.
Credential files and values must not be committed. The prototype's
`SERVER_ADDR` is replaced by this API's `HTTP_ADDR`.

```sh
curl -i http://localhost:8080/api/v1/admin/sheet-bookings \
  -H 'Authorization: Bearer YOUR_OS_ACCESS_TOKEN'
```

PostgreSQL integration tests are opt-in and create/drop temporary schemas;
they never use the application's users table. Point them at a disposable local
database with schema-creation privileges:

```sh
TEST_DATABASE_URL='postgres://chula:localdev@localhost:5432/chula_room_booking?sslmode=disable' \
  go test ./internal/adapter/secondary/repository/postgres -v
```
