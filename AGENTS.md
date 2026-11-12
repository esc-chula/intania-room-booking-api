# Repository Guidelines

## Purpose & Current State

The Chula Room Booking API is a Go modular monolith. Keep it simple, maintainable, and easy to change. Use pragmatic Hexagonal Architecture / Ports & Adapters to separate responsibilities without unnecessary interfaces, duplicate models, mapping layers, or framework abstractions.

Currently implemented: environment configuration, structured logging, optional PostgreSQL connectivity, health/readiness endpoints, development Swagger, CI checks, the core schema, optional Google OAuth sign-in with PostgreSQL users and HttpOnly JWT auth cookies, explicit ADMIN and OS bootstrap commands, and public cached Google Sheets booking endpoints with per-IP rate limiting. Auth/user and sheet-booking domain packages, the PostgreSQL user repository, and the Google Sheets reader with a 60-second cache are implemented; other domain packages remain placeholders. Native booking APIs, sessions, and equipment features are not implemented. Treat the future rules below as design requirements, not existing functionality.

## Project Structure & Module Organization

```text
cmd/api/main.go                              # startup and dependency wiring
internal/config/                             # environment configuration
internal/core/domain/                        # business entities, services, ports
  auth/ user/ sheetbooking/ building/ room/ booking/ audit/ shared/
internal/adapter/primary/http/                # Chi router, handlers, HTTP DTOs
  health/ middleware/
internal/adapter/secondary/repository/postgres/ # direct pgx repositories
internal/adapter/secondary/infrastructure/database/ # pool initialization
internal/adapter/secondary/infrastructure/googlesheets/ # read-only Sheets client
db/migrations/                              # versioned Goose SQL migrations
docs/                                       # generated Swaggo Go/JSON/YAML
.github/workflows/ci.yml                      # automated checks
```

Tests live beside their source files as `*_test.go`. Implemented auth and sheet-booking domain packages use `entity.go` for data types, `port.go` for inbound service and outbound dependency interfaces, and `service.go` for use cases. The user package currently needs only `entity.go`; other domain folders remain placeholders. For new features, useful file names are `entity.go`, `port.go`, and `service.go` in the domain; `handler.go` and `dto.go` in an HTTP feature package; and `room.go` or `booking.go` in the PostgreSQL repository package. Create only files the feature actually needs.

## Architecture & Dependency Rules

```text
HTTP handler → Domain service port → Domain service → Repository/reader port
                                                             ↑
                                             PostgreSQL / Google Sheets adapter
```

- Core domain packages must not import `chi`, `net/http`, `pgx`, Swaggo, configuration/environment packages, or concrete repository implementations.
- `cmd/api/main.go` owns dependency injection and application wiring. Do not add a dependency injection framework.
- HTTP handlers depend on domain service interfaces, while concrete services implement those inbound ports. Keep outbound repository, token, and reader ports in the owning domain package. Bootstrap commands invoke auth domain use cases before repository writes.
- Domain services own business use cases: advance-booking rules, room permissions, opening hours, overrides, submission, cancellation, approval, and conflict checks.
- HTTP handlers decode and validate API input, invoke services, and translate results into HTTP responses. Keep business rules out of handlers and middleware.
- Operational health handlers may use an infrastructure dependency directly; they need no artificial domain service.
- Define ports only when a domain service needs an external dependency. Prefer explicit methods such as `FindApprovedOverlap`, `ListRoomAvailability`, and `ListPendingRequests`; avoid `Repository[T]`, generic CRUD repositories, and base repository frameworks.

A developer should understand a feature without navigating many abstraction layers. Add abstractions only when they solve a demonstrated problem.

## Technology & Representation Choices

Use Chi v5 with ordinary `net/http` handlers, PostgreSQL, direct `pgx/v5` and `pgxpool` access, Goose SQL migrations, Swaggo documentation, and standard `log/slog` logging.

Write explicit, parameterized SQL in PostgreSQL adapters. Do not introduce GORM, another ORM, or sqlc. When database rows match domain entities, scan directly into those entities. Create repository-local row structs only for genuinely different representations, such as joined aggregates or database-specific nullable values.

Keep request DTOs separate when they provide JSON naming, validation, optional inputs, or protection against client-controlled fields. Clients must not set protected fields such as `id`, `status`, `requester_id`, timestamps, or `role`. Response DTOs are useful for hiding fields, formatting, nesting, or API compatibility; they are not mandatory at every boundary. Reuse entities between repository, service, and handler when representations match. Add small conversions only when they differ.

Avoid speculative microservices, Kubernetes, Redis, Kafka, event buses, Huma, interfaces for every type, and factories/mappers for identical structures.

## Build, Test & Development Commands

Use the Go version declared in `go.mod` (currently 1.26.8), Make, a POSIX shell, and Docker Compose. The current module path, `github.com/MadManJJ/room-reservation-api`, is temporary; change it and imports only when the real path is established.

```sh
cp .env.example .env
make tools
make run
```

The public Sheets API runs without PostgreSQL. Set `DATABASE_URL` and
`JWT_SECRET` and run `make db-up` plus `make migrate-up` to enable auth; add both
Google OAuth credentials to enable Google sign-in.

| Command | Purpose |
| --- | --- |
| `make db-up` | Start PostgreSQL 18 and wait for readiness. |
| `make db-down` | Stop Compose while preserving its named data volume. |
| `make tools` | Install pinned Swaggo and Goose binaries into `.bin/`. |
| `make migrate-up` | Apply pending migrations using `DATABASE_URL`. |
| `make migrate-down` | Roll back the latest migration; inspect its impact first. |
| `make bootstrap-admin` | Create the fixed initial ADMIN user only when no ADMIN exists. |
| `make bootstrap-os` | Insert missing OS student IDs and promote listed users without demoting ADMIN. |
| `make run` | Source and export `.env`, then run `go run ./cmd/api`. |
| `make fmt` / `make fmt-check` | Apply or check gofmt formatting. |
| `make vet` / `make test` | Run `go vet ./...` / `go test ./...`. |
| `make build` | Build the API executable. |
| `go build ./...` | Build all packages, as CI also does. |
| `make swagger` / `make swagger-check` | Regenerate docs or check for generated-document drift. |
| `make check` | Run formatting, vet, tests, API build, and Swagger drift checks. |

Run `make tools` before Swagger checks. CI runs `make check` and `go build ./...` without PostgreSQL.

## Coding Style & Naming Conventions

Use idiomatic Go and gofmt, including tabs for indentation. Keep package names lowercase, exported identifiers in `CamelCase`, and unexported identifiers in `camelCase`. Follow existing feature/file naming and use descriptive domain operations. Keep framework concerns in adapters and pass request contexts through service and repository calls. Use the standard library before adding dependencies.

## Testing Guidelines

Use standard `testing` and `net/http/httptest`; no heavy test framework is required. Name tests `Test...` in adjacent `*_test.go` files. Follow existing table-driven tests and subtests where useful.

- Domain services: unit tests for business rules, including pending overlaps, approval conflicts, permissions, and cancellation behavior as features arrive.
- HTTP handlers: `httptest` checks for status codes, response contracts, validation, and service error translation.
- Repositories: real PostgreSQL integration tests for handwritten SQL and constraints. User repository tests opt in with `TEST_DATABASE_URL` and use temporary schemas. Transactions and concurrent approvals need integration tests as those features arrive; preserve database-free unit checks.

No numeric coverage threshold is configured. Run relevant focused tests while developing and `make check` before review. Smoke tests supplement automated tests:

```sh
curl -i http://localhost:8080/health
curl -i http://localhost:8080/ready
curl -i http://localhost:8080/swagger/doc.json
```

With the API running without `DATABASE_URL`, both `/health` and `/ready` return 200. When the API is running with a configured database, stopping PostgreSQL should leave `/health` at 200 and change `/ready` to 503. Restart it with `make db-up` afterward.

## Database & Migration Rules

Use explicit Goose migrations under `db/migrations/`, starting with `00001_init_core_schema.sql`, with `-- +goose Up` and `-- +goose Down` sections. Run migrations separately during development/deployment; never auto-run them at API startup. PostgreSQL enum-value additions are irreversible, so their Down section must be a documented no-op.

The single initial schema includes users, buildings, rooms, booking requests, booking occurrences, room rules, opening hours, date overrides, and audit logs. Users have nullable first name, last name, email, student ID, and phone, and a unique normalized-email index; no password hash or Google subject is stored. The old two-migration history was replaced for a fresh local database. Preserve existing relationships and constraints unless a requirement explicitly changes them.

Important multi-step writes must be atomic. Creating a request and its room reservations belongs in one transaction. Approval must safely coordinate conflict checking, status changes, and audit writes under concurrent requests; an unprotected check followed by an update is insufficient. Keep transaction handling inside repository/application operations, outside HTTP handlers.

## Booking, Availability & Audit Requirements

A `booking_request` contains multiple `booking_rooms`. Each room reservation has its own room, `start_at`, `end_at`, status, attendee count, and remark; do not flatten the model into one shared room or time slot.

Pending reservations may overlap. Conflict becomes authoritative at approval: if another approved reservation overlaps the same room/time, return HTTP 409. OS must cancel the approved reservation before replacing it. Keep this policy in the booking service and repository so it can change without scattering rules across handlers.

Availability should expose approved bookings, pending requests, opening hours, and special date overrides. It is informational: seeing an overlap does not itself prevent submission. The planned 14-day advance rule and other detailed policies must follow the current specification; do not invent missing semantics.

Important OS actions should append audit history: approval, rejection, cancellation of approved bookings, room modification/disabling, and rule/opening-hour changes. Avoid casual update/delete CRUD for audit logs.

## HTTP, Swagger & Health Contracts

Business routes belong under `/api/v1`. When configured, authentication provides protected GET `/api/v1/auth/me` and POST `/api/v1/auth/logout`; Google OAuth uses `/auth/google/login` and `/auth/google/callback` in all environments, while `/auth/google/url` is development-only. The public GET `/api/v1/admin/sheet-bookings` returns filtered, paginated legacy rows from Google Sheets, `/calendar` returns monthly daily counts, and `/weekly` returns room schedules. The three routes share a per-IP, per-instance limit of 600 requests per minute with a burst of 600 and return HTTP 429 when exhausted. Native room and booking routes remain unimplemented. The `/api/v1/admin` prefix provides no authorization: future protected routes must explicitly verify the authenticated user's current OS role through the shared role middleware; that middleware always permits ADMIN as the super-role.

Whenever endpoint behavior changes, update Swaggo comments, run `make swagger`, and commit `docs/docs.go`, `docs/swagger.json`, and `docs/swagger.yaml`. Comments document the contract; business logic stays in code. CI detects stale generation, while tests/review must verify annotations match actual behavior.

Swagger uses base path `/` for the root-level probes and OAuth paths. Future business route annotations must include `/api/v1`. Development UI: `http://localhost:8080/swagger/index.html`; Swagger routes return 404 in production.

| Endpoint | Meaning | Response |
| --- | --- | --- |
| `GET /health` | Process liveness; no database query. | 200, `{"status":"ok"}` |
| `GET /ready` | Ready when PostgreSQL is not configured; otherwise checks it with a two-second ping timeout. | 200, `{"status":"ready"}`; otherwise 503, `{"status":"unavailable"}` |

Both probes use `Cache-Control: no-store`. When `DATABASE_URL` is configured, startup requires a successful PostgreSQL ping. Without it, the public Sheets API starts without auth routes and readiness does not query PostgreSQL. Preserve graceful shutdown: drain HTTP requests within `SHUTDOWN_TIMEOUT`, then close the pool if configured.

## Authentication, Middleware & Logging

Google OAuth is the browser login flow in development and production, using a short-lived, single-use state cookie, PKCE S256, verified ID tokens, and a signed `hd` claim. Only verified `student.chula.ac.th` emails whose local part ends with `21` are eligible. Existing normalized email accounts are not updated, except that OS accounts matched by student ID fill still-blank names from later Google logins. Preloaded OS rows match by student ID and receive only missing profile fields; otherwise a new active USER_A is created. Numeric email local parts become student IDs. The Google subject is not stored. A successful login sets a one-hour HS256 JWT in a host-only, HttpOnly auth cookie and redirects to `FRONTEND_URL`; failures redirect to its `/login` page with a safe error code. Tokens contain no personal data or role. Validate the cookie JWT and load the current active user from PostgreSQL on each protected request; Authorization headers are not accepted and no session storage is used. Disabled/deleted users are rejected and role changes apply immediately. The explicit passwordless ADMIN bootstrap uses a Google-eligible Chula email; the optional OS bootstrap uses `OS_STUDENT_IDS`. JWT_SECRET requires at least 32 non-padding bytes. Logout clears the cookie; password auth, refresh, and server-side revocation are not implemented.

Chi middleware handles infrastructure concerns: request IDs, recovery, structured request logs, cookie authentication, and future authorization. OAuth login and callback redirect; the development URL helper returns JSON. They use Cache-Control: no-store. Domain-specific booking decisions belong in services.

Use structured JSON `log/slog` logs with method, route/path, status, latency, and request ID. Never log OAuth state, authorization codes, tokens, cookies, Authorization headers, database URLs, or request bodies containing personal data. Preserve current query-string redaction, and avoid exposing driver errors containing credentials.

## Configuration & Security

Configuration comes from environment variables. `.env.example` documents local values; the application itself does not load `.env`. `make run` sources it, so use valid shell assignments. Direct `go run` requires exported variables. Migration targets use an exported `DATABASE_URL`, falling back to `.env` when unset.

| Variable | Requirement / default |
| --- | --- |
| `APP_ENV` | `development` or `production`; default `development`. |
| `HTTP_ADDR` | Host:port; default `:8080`. |
| `LOG_LEVEL` | `debug`, `info`, `warn`, or `error`; default `info`. |
| `DATABASE_URL` | Optional for the public Sheets API; when configured, PostgreSQL must be reachable at startup. Required by bootstrap and migration commands. |
| `JWT_SECRET` | Optional while auth is disabled; when configured, at least 32 bytes excluding surrounding whitespace. |
| `BOOTSTRAP_ADMIN_EMAIL` | Google-eligible Chula email required by `make bootstrap-admin` only when no ADMIN exists. |
| `OS_STUDENT_IDS` | Optional comma-separated numeric IDs ending in `21` for `make bootstrap-os`; empty means no OS rows. |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` | Optional; set both with PostgreSQL and `JWT_SECRET` to enable Google OAuth. |
| `GOOGLE_REDIRECT_URL` | Google web-client callback URI; defaults to `http://localhost:8080/auth/google/callback`. |
| `FRONTEND_URL` | Frontend origin for login redirects and credentialed CORS; defaults to `http://localhost:3000` locally and must be HTTPS in production. |
| `AUTH_COOKIE_SAME_SITE` | `none` for cross-site HTTPS (production default) or `lax` for same-site deployment (development default). |
| `GOOGLE_ALLOWED_DOMAIN` / `GOOGLE_ALLOWED_SUFFIX` | Eligibility policy; defaults to `student.chula.ac.th` / `21`. |
| `GOOGLE_SPREADSHEET_ID` / `GOOGLE_SHEET_RANGE` | Required Google Sheets source and range for the live OS booking endpoint. |
| `GOOGLE_APPLICATION_CREDENTIALS` | Optional standard ADC path for local credentials; deployed workloads may use an attached identity. |
| `DB_MAX_CONNS` / `DB_MIN_CONNS` | Defaults 10 / 0; maximum positive, minimum between zero and maximum. |
| `DB_CONNECT_TIMEOUT` / `SHUTDOWN_TIMEOUT` | Positive durations; defaults `5s` / `10s`. |

Keep production credentials out of code and commits. Compose credentials and `sslmode=disable` are local-only; provide suitable PostgreSQL TLS settings in production. The container defaults to production, runs as non-root, and includes no environment files or credentials.

## Commit & Pull Request Guidelines

Recent history uses concise imperative subjects with `feat:`, `docs:`, and `chore:` prefixes, for example `feat: add swagger docs for check health and readiness`. Follow this convention and keep changes focused.

PR descriptions should state the problem and resulting behavior, link relevant issues/specifications when available, and report validation commands/results. Explain schema changes, migration effects, concurrency choices, and API-contract changes when relevant. Include regenerated Swagger artifacts with endpoint changes. No PR template or screenshot requirement currently exists.

## Source Priority & Instructions for Future Agents

Use sources in this order:

1. Current repository code.
2. This `AGENTS.md` architecture and contributor guide.
3. Current API specification.
4. Current DBML/database design.
5. EPIC/business requirements.
6. Older architecture documents.

The API specification, DBML, and EPIC referenced by the team are not currently checked into the repository. When documents disagree with a newer explicit team decision, follow the latest decision and document the deviation. Do not silently redesign API behavior or database relationships. Keep this guide accurate as future features become implemented, and prefer straightforward Go over speculative abstraction.
