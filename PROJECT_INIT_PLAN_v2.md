# Chula Room Booking — Go Backend Initialization Plan (v2)

**For:** Codex (execute in the actual repository)  
**Phase:** 0 — Project initialization only  
**Status:** Ready for implementation  
**Architecture:** Pragmatic Hexagonal / Ports & Adapters  
**Scope:** Room-booking backend foundation; **no business APIs or equipment features**  
**Decisions locked for this phase:** Go, Chi v5, PostgreSQL, **pgx v5 directly (no ORM, no sqlc)**, Goose, Swaggo, `log/slog`. Future authentication uses Chula SSO plus PostgreSQL-backed server-side sessions (not JWT).

> **Codex instruction:** Implement this plan rather than writing a new plan. Inspect the repository first and preserve existing work. Stop when the acceptance checklist passes. Do not begin implementing the room-booking API or database schema.

## 1. Deliverable and strict scope

Create a runnable Go HTTP application with:

- Familiar hexagonal layout: `internal/core/domain`, `internal/adapter/primary/http`, `internal/adapter/secondary`.
- `cmd/api/main.go` for composition, startup, and graceful shutdown.
- Chi router, request ID, panic recovery, structured request logging.
- A real PostgreSQL connection pool using `pgxpool`.
- **Exactly two application endpoints:** `GET /healthz` and `GET /readyz`.
- **Development-only tooling endpoint:** `/swagger/*` with Swaggo-generated Swagger UI and JSON. This is not a business API.
- Valid environment configuration, a local PostgreSQL Compose service, a production-oriented Dockerfile, Makefile, README, `AGENTS.md`, tests and basic GitHub Actions CI.
- A place for future Goose SQL migrations, but no real application tables yet.

**Do not implement:** room/building/user/booking CRUD; booking rules or approval; SSO callback; session creation or session tables; auth/RBAC/CSRF implementation; equipment; migrations for the actual DBML; sample data; fake business services or repository implementations; sqlc; GORM; Huma; JWT; Redis; Kafka; Kubernetes.

The team's **Room Booking API Specification v0.2** defines *future endpoint behavior*, and **DBML v1** defines *future schema*. Neither is to be implemented in Phase 0. This v2 plan supersedes older initialization instructions referring to sqlc or Huma. If the older architecture document recommends sqlc, follow the **pgx-only choice in this plan** for implementation; the API contract is unchanged.

## 2. Technology selection and boundaries

| Responsibility | Choice | Rule |
|---|---|---|
| Runtime | Supported stable Go version compatible with dependencies | Record actual version in `go.mod` and CI; do not assume an uninstalled version |
| HTTP router | `github.com/go-chi/chi/v5` | Use standard `net/http` handlers and middleware |
| DB | PostgreSQL | Managed Postgres in production eventually |
| DB client / driver | `github.com/jackc/pgx/v5` and `pgxpool` | Write parameterized SQL directly in adapters; **no ORM** |
| DB migrations | Goose v3 CLI | SQL files; separate CLI/deployment step; never auto-run on API startup |
| API docs | Swaggo `github.com/swaggo/swag` CLI + `github.com/swaggo/http-swagger/v2` | Documentation comments, normal Chi handlers; generate Swagger 2.0 files |
| Logging | Go standard `log/slog` | JSON logs and sanitized request metadata |
| Authentication (later) | Chula SSO + server-side sessions stored in Postgres | Do not implement in Phase 0 |
| Tests | Go `testing` and `httptest` | No mock framework for this scaffold |
| Local DB | Docker Compose | PostgreSQL only; application can run with `go run` |

**Design rule:** This is pragmatic hexagonal architecture, **not** a mandate for three copies of every entity. In future features, domain types may be scanned directly from `pgx` where practical. `db` field tags are acceptable because they do not add a dependency on pgx. Use small mapping functions only if database and domain representations genuinely differ. HTTP DTOs should exist when needed for API input/output validation or safety; do not expose database-only/sensitive fields. PostgreSQL details remain inside secondary adapters.

**Trade-offs explicitly accepted:** Handwritten pgx SQL is not schema/type-checked by sqlc at generation time. Future repositories must have PostgreSQL integration tests. Swaggo comments can drift from actual code; keep them updated and check generated files in CI. Neither concern requires more framework layers.

## 3. Directory structure

Create the scaffold below. Future feature directories may contain `.gitkeep` files instead of speculative, unused Go types. **Do not create fake `entity.go`, `port.go`, or `service.go` implementations simply to fill folders.** When work starts on a feature, follow the familiar `entity.go`, `port.go`, `service.go` pattern as needed.

```text
<existing-repository>/
├── cmd/
│   └── api/
│       └── main.go                          # Configuration, dependency wiring, startup/shutdown
├── internal/
│   ├── config/
│   │   └── config.go                        # Env parsing and validation
│   ├── core/
│   │   └── domain/
│   │       ├── auth/.gitkeep                # Future SSO and session use cases
│   │       ├── user/.gitkeep
│   │       ├── building/.gitkeep
│   │       ├── room/.gitkeep
│   │       ├── booking/.gitkeep
│   │       ├── audit/.gitkeep
│   │       └── shared/.gitkeep
│   └── adapter/
│       ├── primary/
│       │   └── http/
│       │       ├── router.go                # Chi registration, dev-only Swagger mount
│       │       ├── health/
│       │       │   ├── handler.go           # /healthz, /readyz
│       │       │   └── handler_test.go
│       │       └── middleware/
│       │           └── request_logger.go    # slog request logging
│       └── secondary/
│           ├── repository/
│           │   └── postgres/.gitkeep       # Future pgx repositories; NOT generated models
│           └── infrastructure/
│               └── database/
│                   └── postgres.go          # pgxpool setup / close
├── db/
│   └── migrations/.gitkeep                 # Goose SQL migrations added in next phase
├── docs/                                    # Generated Swaggo docs.go/json/yaml (commit)
├── .github/workflows/ci.yml
├── .env.example
├── .gitignore
├── AGENTS.md
├── Dockerfile
├── compose.yaml
├── Makefile
├── README.md
├── go.mod
└── go.sum
```

`docs/` above is a **generated Go package plus Swagger spec**, not handwritten business documentation. Preserve it in version control so builds do not require running a generator first. **No `sqlc.yaml`, `db/queries/` directory, sqlc imports, or sqlc-generated models.**

**Dependency direction for later features:**

```text
HTTP Handler (primary) -> Domain Service -> Port (domain interface)
                                              ^
                                              |
                                PostgreSQL Repository (secondary)

main.go constructs concrete dependencies and passes them to adapters/services.
```

The health handler is a simple operational HTTP adapter; it does not need a `health` domain. Core domain packages must not import Chi, `net/http`, `pgx`, Swaggo, or concrete secondary adapters. Do not add interfaces for things that don't need them.

## 4. Application startup and graceful shutdown

`cmd/api/main.go` must:

1. Load and validate configuration from environment variables.
2. Create a JSON `slog` logger with configurable log level.
3. Open a `pgxpool.Pool` using `DATABASE_URL`, with bounded startup ping and sensible pool limits. Fail clearly if unavailable.
4. Defer `pool.Close()`.
5. Create the health handler, passing a minimal interface satisfied by the pool.
6. Construct the Chi router, register health routes and (development only) Swagger UI.
7. Configure an `http.Server` with reasonable `ReadHeaderTimeout`, `ReadTimeout`/request bounds, `WriteTimeout` and `IdleTimeout`.
8. Start serving; treat `http.ErrServerClosed` as normal.
9. On SIGINT/SIGTERM, call `Server.Shutdown` with a bounded context (default 10 seconds), then close pool.

Never run migrations automatically at startup. Avoid global singletons, `init()` side effects for your own application configuration, and `log.Fatal` inside internal packages. `main` owns application-level error handling. A blank import of generated Swaggo docs for its registration is normal and acceptable (see §7).

### Environment

Create `.env.example`:

```dotenv
APP_ENV=development
HTTP_ADDR=:8080
LOG_LEVEL=info
DATABASE_URL=postgres://chula:localdev@localhost:5432/chula_room_booking?sslmode=disable
DB_MAX_CONNS=10
DB_MIN_CONNS=0
DB_CONNECT_TIMEOUT=5s
SHUTDOWN_TIMEOUT=10s
```

Use standard `os.Getenv` / `os.LookupEnv` plus explicit parsers; avoid a dedicated env library unless there is a compelling need. The README/Makefile must make local loading easy. `DATABASE_URL` is required; never silently substitute a local database URL in production. The sample password is **local development only**, never production. Validate pool limits, timeouts, `APP_ENV`, `LOG_LEVEL`, and `HTTP_ADDR`; use `.env.example` only as an example, never as a built-in source of runtime secrets. Log errors without printing the full database URL.

## 5. Chi router, middleware and health probes

Build routing in `internal/adapter/primary/http/router.go` (Go package name `httpadapter` is fine):

- Use `chi.NewRouter()` and standard `net/http` functions.
- Add `middleware.RequestID`, `middleware.Recoverer` and custom `slog` request logging; log method, path/route, status, latency, request ID, without logging cookies, authentication headers or request bodies.
- Do not blindly trust `X-Forwarded-For` / proxy headers.
- Do not add global wildcard CORS, session, authentication, RBAC or CSRF middleware during initialization.
- Future business API routes live under `/api/v1`, **but do not register them yet**.

| Method | Path | Behavior |
|---|---|---|
| GET | `/healthz` | Always 200 `{"status":"ok"}` when process is alive; no database call |
| GET | `/readyz` | Ping PostgreSQL with ~2-second timeout; 200 `{"status":"ready"}` or 503 `{"status":"unavailable"}` |
| GET | `/swagger/*` | **Development only**, Swaggo UI/spec assets; must 404 in production |

For health endpoints, set JSON Content-Type and `Cache-Control: no-store`. On readiness failure, log server-side detail but do not reveal DSNs or internal errors in HTTP responses.

In `health/handler.go`, define only a local interface required to test readiness:

```go
// Infrastructure-only dependency, not a business-domain port.
type Pinger interface {
    Ping(context.Context) error
}
```

Use dependency injection so `/readyz` can be unit-tested with a fake pinger. `/healthz` must stay successful if the DB becomes unavailable **after startup**. Unknown application routes return 404.

## 6. PostgreSQL infrastructure: direct pgx (no sqlc)

Implement `internal/adapter/secondary/infrastructure/database/postgres.go`:

- Use `pgxpool.ParseConfig` and `pgxpool.NewWithConfig` from `github.com/jackc/pgx/v5/pgxpool`.
- Configure pool maximum/minimum and connection timeout; ping with bounded context at startup; close cleanly at shutdown.
- Respect TLS settings from the DSN in production. `sslmode=disable` is only for the local Compose setup.
- Never construct SQL from untrusted input. Future SQL uses positional parameters (`$1`, `$2`, etc.).
- Do not implement repository queries, tables, domain entities or transaction helpers at initialization.

**Future repository policy:** Store SQL within the concrete PostgreSQL repository adapter, not HTTP handlers or core domain code. Prefer `QueryRow().Scan(...)`, `pgx.CollectRows(...)` and `pgx.RowToStructByName[...]` where they reduce noise. Scanning into a domain struct is acceptable for straightforward queries; don't force a conversion layer. Add repository-local row structs only for complicated joins, nullable DB representation, or shape differences. Complex booking approval transactions must later be concurrency-safe (lock/constraint design), but **do not implement that logic yet**.

## 7. Swaggo setup — comments, no handler framework

Use **Swaggo**, **not Huma**. It must not change the normal `http.HandlerFunc` / Chi middleware flow.

### Packages and generation

- Install the **stable v1** Swaggo CLI (pin `github.com/swaggo/swag/cmd/swag@v1.16.6` for this plan, not the v2 release candidate).
- Use `github.com/swaggo/http-swagger/v2` to serve docs through Chi.
- Put general Swagger annotations in `cmd/api/main.go` (API title `Chula Room Booking API`, version `0.1.0`, description, `@BasePath /api/v1`). Avoid hardcoding a production host in generated metadata.
- Run from repo root, with the general-info file located in the first `-d` directory:

```bash
swag init \
  -g main.go \
  -d ./cmd/api,./internal/adapter/primary/http \
  --parseInternal \
  -o ./docs
```

- Commit the generated `docs/docs.go`, `docs/swagger.json`, and `docs/swagger.yaml` files. Import the generated docs package using the **actual Go module import path**, e.g. `_ "<actual-go-module>/docs"` in `cmd/api/main.go` so its spec registers.
- In development only, mount `r.Get("/swagger/*", httpSwagger.Handler(httpSwagger.URL("/swagger/doc.json")))` on the Chi router (adapt package alias as needed). Verify `http://localhost:8080/swagger/index.html` and `/swagger/doc.json` work.
- **No sample booking/room endpoints** should be created to make Swagger UI interesting. The initial specification may legitimately have **zero business operations**. `/healthz` and `/readyz` can remain undocumented operational endpoints.
- In production (`APP_ENV=production`) the `/swagger/*` route must not be mounted; don't expose the interactive UI or raw spec by default. The source-generated files may still be compiled in.

### Future annotations policy (put this in `AGENTS.md`)

When implementing/changing an HTTP endpoint later, document it in comments above the handler: `@Summary`, `@Tags`, `@Param`, `@Success`, `@Failure`, `@Router`, etc. Regenerate docs with `make swagger` **in the same change**. Keep annotations and response DTOs consistent with actual handler behavior. No Swaggo annotations in core domain code. **Swagger 2.0 cannot natively model cookie-based session authentication like OpenAPI 3;** describe the secure session cookie and CSRF requirements in API descriptions/README rather than falsely advertising bearer JWT auth. Avoid exposing CSRF-protected write operations in production Swagger UI.

**Generation/CI caveat:** A regenerated-spec diff check catches stale *generated files* but cannot prove annotations actually match handler behavior. Handler tests and code review remain necessary. No full automated contract-test framework is required in Phase 0.

## 8. Goose migrations (prepare, don't execute)

Create `db/migrations/` (a `.gitkeep` is enough). Document how to install a compatible pinned **Goose v3 CLI** and future usage such as:

```bash
# Later, AFTER the real schema migrations exist:
goose -dir db/migrations postgres "$DATABASE_URL" up
```

Run Goose as a separate deployment step. Don't add a `sessions` table, business schema, sample SQL, or migrations just to verify the scaffold. Do not create `sqlc.yaml` or run `sqlc generate`.

## 9. Docker, local development and Makefile

**Compose:** A PostgreSQL-only `db` service, pinned stable major PostgreSQL image (not `latest`), named persistent volume, `pg_isready` healthcheck and local development credentials. Bind published port to `127.0.0.1:5432` when possible. Do not create application tables automatically.

**Dockerfile:** Multi-stage Go build (`./cmd/api`), small non-root runtime image with CA certificates where needed, listens on configured port. Do not copy `.env` or embed credentials. Supply `DATABASE_URL` at runtime. Keep Swagger UI gated by `APP_ENV`, not a separate public service.

**Makefile targets** (simple and portable; document prerequisites):

| Target | Action |
|---|---|
| `make db-up` / `make db-down` | Start/stop local PostgreSQL |
| `make run` | Load local `.env` values and run `go run ./cmd/api` |
| `make fmt` | `gofmt` Go files |
| `make vet` | `go vet ./...` |
| `make test` | `go test ./...` |
| `make build` | `go build ./cmd/api` |
| `make tools` | Install pinned Swaggo CLI (`v1.16.6`); Goose install instructions can be deferred |
| `make swagger` | Run the Swaggo generation command from §7 |
| `make swagger-check` | Regenerate with pinned Swaggo and fail if checked-in `docs/` changes |
| `make check` | Format check + vet + tests + build + swagger-check |

Don't silently swallow errors from commands. Don't require PostgreSQL for unit tests. Use `go mod tidy` and commit `go.mod` / `go.sum` as normal.

### CI

Add a minimal GitHub Actions workflow on push/PR to:

1. Install the Go version compatible with `go.mod`.
2. Install the **same pinned Swaggo CLI** version as Makefile.
3. Check formatting, `go vet ./...`, `go test ./...`, `go build ./...`.
4. Run `make swagger-check` and fail if generated documentation is stale (including newly generated/untracked files).

No database service or actual migrations are necessary in CI for Phase 0. Avoid introducing test frameworks, security scanners, deployment pipelines, or PR automation during this scaffold unless already present in the repository.

## 10. Test and verify

Implement tests for:

1. `/healthz` -> 200 JSON `{"status":"ok"}`, does **not** ping database.
2. `/readyz` -> 200 JSON `{"status":"ready"}` if injected fake pinger succeeds.
3. `/readyz` -> 503 JSON `{"status":"unavailable"}` if fake pinger fails or its context times out.
4. Unknown routes -> 404.
5. `/swagger/doc.json` and `/swagger/index.html` are served in `development` and not served in `production`; generated JSON should be valid, even if no API paths exist.
6. Config rejects missing DB URL, invalid pool values, invalid duration, invalid `APP_ENV`, etc.
7. `go test ./...`, `go vet ./...`, `go build ./...`, formatting check and docs drift check all pass without PostgreSQL.

If Docker is available, perform a real smoke test:

```bash
make db-up
make run
# In another terminal:
curl -i http://localhost:8080/healthz              # 200
curl -i http://localhost:8080/readyz               # 200
curl -i http://localhost:8080/swagger/doc.json     # 200 in development
curl -i http://localhost:8080/api/v1/rooms          # 404 (not implemented)
```

After startup, temporarily stop PostgreSQL: `/healthz` should remain 200; `/readyz` should change to 503. Verify SIGINT/SIGTERM graceful shutdown. If Docker isn't available, report that the smoke test was skipped; **don't claim it passed**.

## 11. Documentation and maintainability rules

Create a concise `README.md` covering: purpose, stack, architecture diagram, prerequisites, `make db-up`, `.env` setup, `make run`, health endpoints, Swagger URL, `make swagger`, tests, Docker build, future Goose migrations, and explicit Phase 0 limitations.

Create a concise `AGENTS.md` telling Codex/future contributors:

- Use familiar pragmatic hexagonal boundaries; keep domain import-free of HTTP, Swaggo, pgx, environment config and concrete adapters.
- **No sqlc, Huma, ORM, DTO explosion, generic repositories, unnecessary interfaces, or DI frameworks.**
- Use Chi / `net/http` handlers and pgx directly in Postgres repository adapters.
- For future feature folders use `entity.go`, `port.go`, `service.go`, and `handler.go`/`dto.go` when needed, not as mandatory empty boilerplate.
- Write SQL with parameters; future integration tests must execute real PostgreSQL queries.
- For any API change, update Swaggo comments, rerun `make swagger`, and update API tests.
- Do not add new APIs/schema/equipment during this initialization task.
- Preserve the team's DBML and room API spec as future functional sources of truth; document any later departures.

`.gitignore` must exclude `.env`, compiled binaries, tests' generated output, and editor files **without** ignoring `.env.example`, migration SQL, or committed generated Swagger docs.

## 12. Definition of done / acceptance checklist

- [ ] Existing repository inspected; no destructive rewrites or nested project directory.
- [ ] `go.mod` uses repo's real module path (or a clearly documented temporary local path if remote not known).
- [ ] Hexagonal directory skeleton exists; no made-up business entities/services or equipment directory.
- [ ] Main wires config -> logger -> pgxpool -> health handler -> Chi -> HTTP server.
- [ ] Direct `pgx` is used; **no sqlc config/code**, ORM, Huma, or JWT.
- [ ] `/healthz` and `/readyz` work and have different dependency semantics.
- [ ] Development `/swagger/index.html` and `/swagger/doc.json` work; production disables docs route.
- [ ] Swaggo annotations in main, generator command, generated docs committed and imported correctly.
- [ ] Makefile has `swagger` and `swagger-check`; CI runs the drift check with the same pinned tool.
- [ ] Config, DB connection, request logging, timeouts, graceful shutdown function correctly.
- [ ] Compose, Dockerfile, `.env.example`, README, `AGENTS.md`, tests and CI are present.
- [ ] No business CRUD/SSO/sessions/schema/booking approval/equipment added.
- [ ] Format, vet, test, build and Swagger drift check pass; smoke test results reported accurately.
- [ ] Report exact files modified, commands executed, successes and any limitations.

## 13. Execution steps for Codex

1. **Inspect first:** list existing project files, read the current `go.mod` (if any), inspect Git remote, and preserve established work.
2. Determine a valid module path without inventing a real GitHub organization.
3. Create minimal folders/placeholders, config, pgxpool, main, Chi router and health handlers.
4. Add Swaggo general annotations and generated spec, mount docs only for development, and test both environments.
5. Add PostgreSQL Compose, Dockerfile, Makefile targets, `.env.example`, README, `AGENTS.md`, `.gitignore` and CI.
6. Format; run tests, vet, build, Swagger drift check and local Docker smoke tests where possible.
7. Summarize work and **stop at Phase 0**.

---

### Technical references

- Chi router: https://github.com/go-chi/chi
- pgx / pgxpool: https://github.com/jackc/pgx
- Goose: https://github.com/pressly/goose
- Swaggo annotations + CLI: https://github.com/swaggo/swag
- Chi-compatible Swagger UI: https://github.com/swaggo/http-swagger

### Team references for future phases

- **Room Booking API Specification v0.2** — future route contracts (not yet implemented).
- **DBML v1** — existing database entities/relationships (not yet migrated).
- **EPIC Room Booking System** — business requirements and unresolved OS rules.

