# Repository Guidelines

## Purpose & Current State

The Chula Room Booking API is a Go modular monolith. Keep it simple, maintainable, and easy to change. Use pragmatic Hexagonal Architecture / Ports & Adapters to separate responsibilities without unnecessary interfaces, duplicate models, mapping layers, or framework abstractions.

Currently implemented: environment configuration, structured logging, PostgreSQL connectivity, health/readiness endpoints, development Swagger, CI checks, the core schema, and email/password registration/login with stateless JWT authentication. Auth/user domain packages and the PostgreSQL user repository are implemented; other domain packages remain placeholders. Chula SSO, booking APIs, sessions, and equipment features are not implemented. Treat the future rules below as design requirements, not existing functionality.

## Project Structure & Module Organization

```text
cmd/api/main.go                              # startup and dependency wiring
internal/config/                             # environment configuration
internal/core/domain/                        # business entities, services, ports
  auth/ user/ building/ room/ booking/ audit/ shared/
internal/adapter/primary/http/                # Chi router, handlers, HTTP DTOs
  health/ middleware/
internal/adapter/secondary/repository/postgres/ # direct pgx repositories
internal/adapter/secondary/infrastructure/database/ # pool initialization
db/migrations/                              # versioned Goose SQL migrations
docs/                                       # generated Swaggo Go/JSON/YAML
.github/workflows/ci.yml                      # automated checks
```

Tests live beside their source files as `*_test.go`. For new features, useful file names are `entity.go`, `port.go`, and `service.go` in the domain; `handler.go` and `dto.go` in an HTTP feature package; and `room.go` or `booking.go` in the PostgreSQL repository package. Create only files the feature actually needs.

## Architecture & Dependency Rules

```text
HTTP handler → Domain service → Repository port
                                     ↑
                              PostgreSQL adapter
```

- Core domain packages must not import `chi`, `net/http`, `pgx`, Swaggo, configuration/environment packages, or concrete repository implementations.
- `cmd/api/main.go` owns dependency injection and application wiring. Do not add a dependency injection framework.
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

Use the Go version declared in `go.mod` (currently 1.26.8), Make, a POSIX shell, and Docker Compose. The current module path, `example.com/chula-room-booking`, is temporary; change it and imports only when the real path is established.

```sh
cp .env.example .env
make db-up
make tools
make migrate-up
make run
```

| Command | Purpose |
| --- | --- |
| `make db-up` | Start PostgreSQL 18 and wait for readiness. |
| `make db-down` | Stop Compose while preserving its named data volume. |
| `make tools` | Install pinned Swaggo and Goose binaries into `.bin/`. |
| `make migrate-up` | Apply pending migrations using `DATABASE_URL`. |
| `make migrate-down` | Roll back the latest migration; inspect its impact first. |
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
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/readyz
curl -i http://localhost:8080/swagger/doc.json
```

With the API already running, stopping PostgreSQL should leave `/healthz` at 200 and change `/readyz` to 503. Restart it with `make db-up` afterward.

## Database & Migration Rules

Use explicit Goose migrations under `db/migrations/`, following names such as `00001_init_core_schema.sql` and `00002_add_password_auth.sql`, with `-- +goose Up` and `-- +goose Down` sections. Run migrations separately during development/deployment; never auto-run them at API startup.

The initial schema includes users, buildings, rooms, booking requests, booking occurrences, room rules, opening hours, date overrides, and audit logs. Password authentication adds nullable password hashes and a unique normalized-email index in a separate migration; existing email collisions fail migration rather than merge users. Preserve existing relationships and constraints unless a requirement explicitly changes them.

Important multi-step writes must be atomic. Creating a request and its room reservations belongs in one transaction. Approval must safely coordinate conflict checking, status changes, and audit writes under concurrent requests; an unprotected check followed by an update is insufficient. Keep transaction handling inside repository/application operations, outside HTTP handlers.

## Booking, Availability & Audit Requirements

A `booking_request` contains multiple `booking_rooms`. Each room reservation has its own room, `start_at`, `end_at`, status, attendee count, and remark; do not flatten the model into one shared room or time slot.

Pending reservations may overlap. Conflict becomes authoritative at approval: if another approved reservation overlaps the same room/time, return HTTP 409. OS must cancel the approved reservation before replacing it. Keep this policy in the booking service and repository so it can change without scattering rules across handlers.

Availability should expose approved bookings, pending requests, opening hours, and special date overrides. It is informational: seeing an overlap does not itself prevent submission. The planned 14-day advance rule and other detailed policies must follow the current specification; do not invent missing semantics.

Important OS actions should append audit history: approval, rejection, cancellation of approved bookings, room modification/disabling, and rule/opening-hour changes. Avoid casual update/delete CRUD for audit logs.

## HTTP, Swagger & Health Contracts

Business routes belong under `/api/v1`. Authentication provides POST `/api/v1/auth/register`, POST `/api/v1/auth/login`, and protected GET `/api/v1/auth/me`. Room and booking routes remain unimplemented. OS routes may use `/api/v1/admin`, but the prefix provides no authorization: explicitly verify the authenticated user's current OS role.

Whenever endpoint behavior changes, update Swaggo comments, run `make swagger`, and commit `docs/docs.go`, `docs/swagger.json`, and `docs/swagger.yaml`. Comments document the contract; business logic stays in code. CI detects stale generation, while tests/review must verify annotations match actual behavior.

Swagger currently uses base path `/` for the root-level probes. Future business route annotations must include `/api/v1`. Development UI: `http://localhost:8080/swagger/index.html`; Swagger routes return 404 in production.

| Endpoint | Meaning | Response |
| --- | --- | --- |
| `GET /healthz` | Process liveness; no database query. | 200, `{"status":"ok"}` |
| `GET /readyz` | PostgreSQL readiness with a two-second ping timeout. | 200, `{"status":"ready"}`; otherwise 503, `{"status":"unavailable"}` |

Both probes use `Cache-Control: no-store`. Startup requires a successful PostgreSQL ping. Preserve graceful shutdown: drain HTTP requests within `SHUTDOWN_TIMEOUT`, then close the pool.

## Authentication, Middleware & Logging

The latest team decision replaces the older server-side session design with stateless JWTs. Email/password auth is available in all environments; registration creates active USER_A users. Passwords use bcrypt cost 12 and accept 8-72 bytes. Login and registration issue one-hour HS256 bearer tokens with UUID subject, issue/expiry times, issuer, and audience. Tokens contain no personal data or role. Validate tokens and load the current active user from PostgreSQL on each protected request; no session storage is used. Disabled/deleted users are rejected and role changes apply immediately. JWT_SECRET requires at least 32 non-padding bytes. Client logout discards the token; refresh, server-side revocation, password reset, email verification, and OAuth are not implemented. Future Chula SSO/OAuth can reuse user resolution and JWT issuance.

Chi middleware handles infrastructure concerns: request IDs, recovery, structured request logs, bearer authentication, and future authorization. Auth handlers reject unknown/protected fields and bodies over 16 KiB, return JSON errors, and use Cache-Control: no-store. Domain-specific booking decisions belong in services.

Use structured JSON `log/slog` logs with method, route/path, status, latency, and request ID. Never log session cookies, Authorization headers, passwords, database URLs, or request bodies containing personal data. Preserve current query-string redaction, and avoid exposing driver errors containing credentials.

## Configuration & Security

Configuration comes from environment variables. `.env.example` documents local values; the application itself does not load `.env`. `make run` sources it, so use valid shell assignments. Direct `go run` requires exported variables. Migration targets use an exported `DATABASE_URL`, falling back to `.env` when unset.

| Variable | Requirement / default |
| --- | --- |
| `APP_ENV` | `development` or `production`; default `development`. |
| `HTTP_ADDR` | Host:port; default `:8080`. |
| `LOG_LEVEL` | `debug`, `info`, `warn`, or `error`; default `info`. |
| `DATABASE_URL` | Required PostgreSQL connection string. |
| `JWT_SECRET` | Required signing secret; at least 32 bytes excluding surrounding whitespace. |
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
