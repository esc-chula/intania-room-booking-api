# Contributor instructions

Use pragmatic hexagonal boundaries: HTTP handlers call domain services/ports;
PostgreSQL adapters implement the ports; main wires concrete dependencies.
Core domain code must not import HTTP, Chi, Swaggo, pgx, environment configuration,
or concrete adapters. Keep SQL inside PostgreSQL adapters and use parameters.
Use Chi/net/http handlers and pgx directly. No sqlc, Huma, ORM, generic repositories,
DI frameworks, DTO explosion, or unnecessary interfaces.

Future features may use entity.go, port.go, service.go, handler.go, and dto.go
when needed; do not add empty Go boilerplate. Scan into domain structs where
practical; add mapping only when representations differ. Future repository
integration tests must execute real PostgreSQL queries. Run Goose separately;
never run migrations automatically on API startup.

For every API change, update handler comments (@Summary, @Tags, @Param,
@Success, @Failure, @Router as appropriate), regenerate with make swagger in
the same change, and update handler tests. No Swagger annotations in core.
Swagger 2.0 cannot natively describe cookie session authentication: document
secure session cookies and CSRF requirements in descriptions/README, never
advertise bearer JWT. Keep Swagger UI development-only; do not expose
CSRF-protected write operations in a production UI. Generated-file drift
checks do not prove annotations match behavior; retain tests and review.

This initialization is Phase 0 only: no business APIs, schema, equipment, SSO,
session implementation, or booking approval. Preserve PROJECT_INIT_PLAN_v2.md
and the team's DBML v1, Room Booking API Specification v0.2, and EPIC as future
functional sources of truth; document later departures. These team references
are not currently present in this repository.

Run make tools and make check before completing changes. Commit generated
docs/docs.go, docs/swagger.json, and docs/swagger.yaml with their source changes.
