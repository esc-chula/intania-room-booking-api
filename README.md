# Chula Room Booking API

Go backend foundation using Chi v5, pgx v5/pgxpool, PostgreSQL, log/slog,
Swaggo v1, and Goose SQL migrations. The single initial migration defines users,
buildings, rooms, bookings, room scheduling, and audit logs. Optional Google OAuth
creates or finds users and sets one-hour JWT auth cookies. Explicit
commands bootstrap ADMIN and OS users. Public endpoints read legacy
booking rows live from Google Sheets. Native booking APIs and equipment features
are not implemented yet.

```text
HTTP handler -> Domain Service port -> Domain service -> Domain repository/reader port
                                                             ^
                                                             |
                                             PostgreSQL / Google Sheets adapter

cmd/api wires config -> logger -> optional pgxpool/auth -> services -> handlers -> Chi
```

Implemented auth and sheet-booking domain packages keep entities in `entity.go`,
inbound and outbound ports in `port.go`, and use cases in `service.go`. HTTP
handlers depend on the service ports; PostgreSQL, token, and Google Sheets
adapters implement the outbound ports. The ADMIN and OS bootstrap commands also
call auth domain use cases. The user package currently needs only `entity.go`;
other domain folders remain placeholders. Operational health handlers need no
domain service.
Repositories use parameterized pgx SQL directly, without an ORM
or sqlc. The temporary Go module path is `github.com/MadManJJ/room-reservation-api` because
this repository has no Git remote; replace it and its imports when the real
module path is known.

## Local development

Prerequisites: Go 1.26.8, Make, a POSIX shell, Google Cloud CLI, a GCP project
with the Google Sheets API enabled, and curl for smoke checks. Docker with
Compose is only needed for the optional local PostgreSQL database. The Makefile
uses standard gofmt, mktemp, and diff utilities.

1. Choose a GCP project and a Google spreadsheet you are allowed to read. The
   project must have billing enabled where required and the Sheets API enabled:

   ```sh
   export GCP_PROJECT_ID=your-gcp-project-id
   gcloud auth login
   gcloud services enable sheets.googleapis.com --project "$GCP_PROJECT_ID"
   ```

   Enabling the API requires appropriate project permissions. Share the
   spreadsheet with a service account as **Viewer**. For this project's personal
   test deployment, its runtime account is
   `room-booking-api@intania-room-booking-test.iam.gserviceaccount.com`.

2. Set up local Application Default Credentials (ADC) by impersonating that
   service account. Your Google user needs `roles/iam.serviceAccountTokenCreator`
   on the service account; an administrator can grant it if needed. Do not
   download a service-account key.

   ```sh
   export SHEETS_SERVICE_ACCOUNT=your-service-account@your-gcp-project-id.iam.gserviceaccount.com
   gcloud config set project "$GCP_PROJECT_ID"
   gcloud auth application-default login --impersonate-service-account "$SHEETS_SERVICE_ACCOUNT"
   ```

   The `gcloud` login and ADC are separate credentials. Impersonation lets the
   Go client request its read-only Sheets scope; ordinary user ADC defaults to
   Cloud Platform scopes and may fail with insufficient authentication scopes.
   See [Google's ADC documentation](https://docs.cloud.google.com/docs/authentication/application-default-credentials).

3. Copy the configuration, set the required spreadsheet ID and range in `.env`,
   and run the API:

```sh
cp .env.example .env
make tools
make run
```

Open http://localhost:8080/swagger/index.html in development, or test:

```sh
curl -i http://localhost:8080/health
curl -i http://localhost:8080/ready
curl -i http://localhost:8080/api/v1/admin/sheet-bookings
```

The spreadsheet ID is the part between `/d/` and `/edit` in its URL. Set
`GOOGLE_SHEET_RANGE` to the actual tab and range; keep quotes around values with
spaces or shell-sensitive characters because `make run` sources `.env`.
If a Sheets request returns 502, check the API is enabled, the impersonated
account can view the Sheet, and your user can impersonate it.

The public Sheets API does not need PostgreSQL. For the optional PostgreSQL-backed
auth and OAuth features, set `DATABASE_URL` and `JWT_SECRET`, then run `make
db-up` and `make migrate-up` before starting the API.

`make run` sources the local `.env` as shell assignments and exports them.
When running `go run ./cmd/api` directly, export environment variables yourself.
The application does not read `.env` or fall back to `.env.example`.
`GOOGLE_SPREADSHEET_ID` and `GOOGLE_SHEET_RANGE` are required. PostgreSQL is
optional for the public Sheets API: leave `DATABASE_URL` empty to start without
it. Auth routes require `DATABASE_URL` and `JWT_SECRET`; add both
`GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET` to enable Google sign-in. OAuth
credentials must be set together. Configured PostgreSQL connection failures
still stop startup. Set `FRONTEND_URL` to the frontend origin when auth is enabled.
`BOOTSTRAP_ADMIN_EMAIL` is required when running
`make bootstrap-admin` against a database without an ADMIN. JWT_SECRET must contain at least
32 bytes excluding surrounding whitespace; use an independently generated
random secret when enabling auth (for example, `openssl rand -hex 32`). Other
settings default to the values in `.env.example`.
Supported `APP_ENV` values are `development` and `production`; log levels are
`debug`, `info`, `warn`, and `error`. Pool maximum must be positive, minimum
between zero and maximum, and both timeouts positive. HTTP_ADDR is host:port.

Compose runs PostgreSQL 18 only, binds localhost:5432, and retains data in a
named volume. `chula`/`localdev` and `sslmode=disable` are for local development
only. Supply production credentials and appropriate PostgreSQL TLS settings at
runtime; the driver respects the DSN. `make db-down` stops Compose and preserves
the volume. No migrations run during startup.

## Probes and Swagger

- `GET /health`: 200 `{"status":"ok"}`; it does not query PostgreSQL.
- `GET /ready`: 200 `{"status":"ready"}` when PostgreSQL is not configured;
  otherwise it pings PostgreSQL with a two-second timeout and returns 503 when
  unavailable. Both probes return JSON with `Cache-Control: no-store`.
- Development docs: http://localhost:8080/swagger/index.html and
  http://localhost:8080/swagger/doc.json. Both return 404 in production.
- All unknown routes, including `/api/v1/rooms`, return 404. Business routes
  use `/api/v1`; the development OAuth browser flow uses the requested
  `/auth/google` paths. Swagger's base path is `/` so both are accurate.

The process fails startup if configured PostgreSQL is unavailable. With no
`DATABASE_URL`, it logs that PostgreSQL is disabled and continues without the
auth and OAuth routes. With no Google OAuth credentials, it logs that OAuth is
disabled. SIGINT/SIGTERM drain HTTP requests within SHUTDOWN_TIMEOUT (10 seconds
by default), then close the pool when one is configured. HTTP header/read/write/idle timeouts are 5/10/15/60 seconds. Request logs
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
curl -i http://localhost:8080/health
curl -i http://localhost:8080/ready
curl -i http://localhost:8080/swagger/doc.json
curl -i http://localhost:8080/api/v1/rooms
```

With `DATABASE_URL` unset, `/ready` remains 200 because PostgreSQL is not a
configured dependency. To exercise the configured database probe, set
`DATABASE_URL`, start the Compose database, and then stop it; `/ready` returns
503 while `/health` remains 200.

Stop the API with Ctrl-C or SIGTERM. GitHub Actions runs checks without a DB.

## Container and migrations

```sh
docker build -t chula-room-booking-api .
docker run --rm -p 127.0.0.1:8080:8080 \
  -e DATABASE_URL='postgres://USER:PASSWORD@DB_HOST:5432/DB_NAME?sslmode=verify-full' \
  -e JWT_SECRET="$JWT_SECRET" \
  -e GOOGLE_CLIENT_ID="$GOOGLE_CLIENT_ID" \
  -e GOOGLE_CLIENT_SECRET="$GOOGLE_CLIENT_SECRET" \
  -e GOOGLE_REDIRECT_URL='https://api.example.com/auth/google/callback' \
  -e FRONTEND_URL='https://frontend.example.com' \
  -e AUTH_COOKIE_SAME_SITE=none \
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

The single initial migration has nullable first name, last name, email, student
ID, and phone columns on users, plus a unique index on trimmed, lowercase emails.
It contains no password or Google subject column. Replacing the old migrations
requires a fresh local database; `docker compose down -v` deletes the named
volume and all its data. Then run `make db-up` and `make migrate-up`. The schema
excludes equipment. The team's API v0.2, DBML v1, and EPIC are not checked in.

## Authentication

Google login and callback are available in development and production. The
manual URL helper is development-only. Set the Google web OAuth client's
authorized redirect URI exactly to `GOOGLE_REDIRECT_URL`, which points to the
backend callback. `FRONTEND_URL` is the frontend origin to visit afterward.
Production requires HTTPS for both URLs.

| Endpoint | Input | Success |
| --- | --- | --- |
| `GET /auth/google/url` | Browser cookie | JSON `auth_url` and temporary cookie |
| `GET /auth/google/login` | Browser | 302 redirect to Google |
| `GET /auth/google/callback` | Google code/state, browser cookie | Set auth cookie; redirect to frontend |
| `GET /api/v1/auth/me` | Auth cookie | 200, current user |
| `POST /api/v1/auth/logout` | Browser | 204; clear auth cookie |

The browser flow uses a ten-minute, single-use state bound to an HttpOnly cookie
and PKCE S256. The API verifies Google's signed ID token, issuer, audience, and
expiry. Only verified `student.chula.ac.th` accounts whose email local part
**ends with `21`** are eligible. The `hd` request parameter is only an account
selection hint; the signed `hd` and email claims are checked on callback.
Google access tokens are never returned or stored.

An existing normalized email is returned without changing the database row.
A numeric email local part can match a preloaded OS `student_id`; its first login
fills email and available first/last names while preserving OS. A new account
gets `USER_A`, and a numeric local part becomes its student ID. Ambiguous email
and student ID matches redirect to the frontend login page with an error code.
The Google subject is not stored. Optional profile names may be absent.

The callback sets a one-hour HS256 application JWT in a host-only, HttpOnly
cookie and redirects to `FRONTEND_URL`. Errors redirect to its `/login` page
with a simple `auth_error` code. No token appears in the redirect URL or body.
The JWT contains user ID, issue/expiry times, issuer, and audience, but no role or personal data.
Protected routes reload the current active user from PostgreSQL on every
request, so disabling a user or changing a role takes effect immediately.
The API reads only the auth cookie; Authorization headers are ignored. Logout
clears the cookie but does not revoke an already issued JWT. Responses use
`Cache-Control: no-store`; the callback blocks referrers. There is no refresh token.

```sh
curl -i http://localhost:8080/auth/google/url
# For a full test, open this in the browser that will receive the callback:
# http://localhost:8080/auth/google/login
# After browser sign-in, call from the frontend:
# fetch('http://localhost:8080/api/v1/auth/me', {credentials: 'include'})
```

The URL returned to curl cannot normally be opened in an unrelated browser:
the browser needs the same state cookie. For a cross-site frontend and API,
use `AUTH_COOKIE_SAME_SITE=none` with HTTPS and `credentials: 'include'`. The API
allows credentialed CORS only from `FRONTEND_URL`. Some browsers block cross-site
cookies even with these settings; a same-site reverse proxy can use
`AUTH_COOKIE_SAME_SITE=lax` for reliable cookie delivery.
In development, Swagger can also send logout requests from the API origin in
`GOOGLE_REDIRECT_URL`; open Swagger using that same host and port.

## Bootstrap administrator

Set `BOOTSTRAP_ADMIN_EMAIL` to a Google-eligible Chula email, then run the
explicit `make bootstrap-admin` command. It creates an active ADMIN with
nullable profile fields only if no ADMIN exists. A second run does nothing.
If that email belongs to a non-admin account, the command fails rather than
promoting it. ADMIN is accepted by every role-guarded route, but can sign in
only through the development Google flow at this stage.

`OS_STUDENT_IDS` is an optional comma-separated list, for example
`6730000021,6720000021`. `make bootstrap-os` inserts missing OS rows with
nullable profile fields and promotes existing listed users to OS; it never
demotes ADMIN. Repeated runs are safe. An empty list is a no-op. This command
is not run during API startup or migration.

```sh
make migrate-up
make bootstrap-admin
make bootstrap-os
```

## OS spreadsheet bookings

`GET /api/v1/admin/sheet-bookings` returns `{ "items": [], "page": 1,
"page_size": 20, "total": 0 }`. It accepts `page` (default 1), `page_size`
(default 20, maximum 100), `year` and `month` together, `building`, and
`status`. Month filtering includes rows whose date ranges overlap the month.
`GET /api/v1/admin/sheet-bookings/calendar` requires `year` and `month` and
accepts an optional `building`. It returns sparse daily counts for `approved`,
`cancelled`, `submitted`, `timeline`, `needs_revision`, and `conflicted`.
The conflict count includes active rows that overlap another active booking
for the same room and daily time window; it is additional to status counts.
Both endpoints share a 60-second in-memory Sheet cache. A failed refresh
returns 502. The response uses `Cache-Control: no-store`.

`GET /api/v1/admin/sheet-bookings/weekly` returns bookings grouped under every
room in `rooms.json`, including rooms with an empty `bookings` array. Optional
`start_date` and `end_date` are inclusive `YYYY-MM-DD` dates covering at most
seven days. Both omitted means today through six days later in Bangkok time;
`start_date` alone also covers seven days. `end_date` alone is invalid. An
optional `building` filter accepts `ENG 3`, `ENG 100`, or `ENG 4` and still
returns empty rooms in that building. Each booking has the Sheet ID from column
A (or `"-1"` if blank), title from column C, Bangkok timestamps, lowercase
status, and an active-overlap conflict flag. Multi-day rows appear once per day.
Unmatched rooms and invalid dates or times are omitted. This endpoint shares
the same 60-second Sheet cache, OS/ADMIN authorization, and error behavior.

The Sheet range must begin at column A and include column AE (for example
`จองห้อง!A9:AM`). Booking fields come from A, F, N, P, Q, R, S, and AE.
Building filters accept ENG building keys from `rooms.json` and match the room
prefix before `/`. Unprefixed rooms appear only without a building filter.

The endpoint requires the auth cookie for a currently active OS
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
# From the signed-in frontend, use fetch with credentials: 'include'.
```

PostgreSQL integration tests are opt-in and create/drop temporary schemas;
they never use the application's users table. Point them at a disposable local
database with schema-creation privileges:

```sh
TEST_DATABASE_URL='postgres://chula:localdev@localhost:5432/chula_room_booking?sslmode=disable' \
  go test ./internal/adapter/secondary/repository/postgres -v
```
