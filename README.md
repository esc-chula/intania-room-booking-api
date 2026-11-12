# Intania Room Booking API

This API reads legacy room bookings from Google Sheets. The three endpoints below are read-only and require an active authenticated account of any role. Despite the `/admin` path segment, they do not require an OS or ADMIN role. Without PostgreSQL-backed authentication, they return `503 Service Unavailable`.

## Local development

You need Go 1.26.8, Make, Docker Compose, the Google Cloud CLI, a Google Cloud project with the Google Sheets API enabled, Google OAuth credentials, and a spreadsheet that the service account can view.

1. Sign in to `gcloud`, then create local Application Default Credentials (ADC) by impersonating the service account. Your Google account needs permission to impersonate it (for example, `roles/iam.serviceAccountTokenCreator`), and the spreadsheet must be shared with the service account as a Viewer.

   ```sh
   gcloud auth login
   gcloud auth application-default login --impersonate-service-account=YOUR_SERVICE_ACCOUNT@YOUR_PROJECT.iam.gserviceaccount.com
   ```

   `gcloud auth login` alone does not provide credentials to the Go API. It uses ADC to call Sheets with read-only access. See [Google's ADC documentation](https://docs.cloud.google.com/docs/authentication/application-default-credentials). If `GOOGLE_APPLICATION_CREDENTIALS` is set in your shell, ADC uses that file first; unset it to use the impersonated credentials above.

2. Copy the environment file and set the spreadsheet, database, JWT, and Google OAuth values in `.env`:

   ```sh
   cp .env.example .env
   ```

   ```sh
   GOOGLE_SPREADSHEET_ID=your-spreadsheet-id
   GOOGLE_SHEET_RANGE='จองห้อง!A9:AM'
   DATABASE_URL=postgres://chula:localdev@localhost:5432/chula_room_booking?sslmode=disable
   JWT_SECRET=replace-with-at-least-32-random-bytes
   GOOGLE_CLIENT_ID=your-google-client-id
   GOOGLE_CLIENT_SECRET=your-google-client-secret
   ```

   The spreadsheet ID is between `/d/` and `/edit` in its URL. Set the range to the correct tab and data rows; it must start at column A and include column AE. Keep quotes around a range containing spaces or shell-sensitive characters.

3. Start PostgreSQL, apply migrations, and start the API:

   ```sh
   make db-up
   make migrate-up
   make run
   ```

   `make run` loads `.env`. If you use `go run ./cmd/api` directly, export those variables yourself. The default address is `http://localhost:8080`.

## Google Sheets booking endpoints

View the interactive API docs at [http://localhost:8080/swagger/index.html](http://localhost:8080/swagger/index.html) while the API is running locally.

All three endpoints require the `room_auth` cookie set by Google sign-in and return JSON from a shared 60-second Sheet cache. Missing or invalid authentication returns `401`; unavailable authentication returns `503`; invalid queries return `400`; Sheet fetch failures return `502`. For a `502`, check Sheet access, the enabled Sheets API, and the ID and range in `.env`.

### List bookings

`GET /api/v1/admin/sheet-bookings` returns `{ "items": [], "page": 1, "page_size": 20, "total": 0 }`. Filter by `year` and `month` together, `building`, or `status`. Pagination defaults to page 1 with 20 items; `page_size` can be at most 100.

```sh
curl --cookie 'room_auth=YOUR_AUTH_COOKIE' 'http://localhost:8080/api/v1/admin/sheet-bookings?year=2026&month=11&page=1&page_size=20'
```

### Monthly calendar

`GET /api/v1/admin/sheet-bookings/calendar` requires `year` and `month` and accepts optional `building`. It returns daily booking status and conflict counts in `{ "year": 2026, "month": 11, "days": [...] }`.

```sh
curl --cookie 'room_auth=YOUR_AUTH_COOKIE' 'http://localhost:8080/api/v1/admin/sheet-bookings/calendar?year=2026&month=11&building=ENG%203'
```

### Weekly room schedule

`GET /api/v1/admin/sheet-bookings/weekly` returns `{ "start_date": "...", "end_date": "...", "rooms": [...] }`, including rooms without bookings. Optional `start_date` and `end_date` use `YYYY-MM-DD` and cover at most seven days; the default is today through six days later in Bangkok. `end_date` requires `start_date`. Optional `building` filters the rooms.

The three Sheet endpoints share a per-IP limit of 600 requests per minute with a burst of 600 on each API instance. Excess requests return HTTP 429 with `Retry-After: 1`. The in-memory limit resets when an instance restarts.

```sh
curl --cookie 'room_auth=YOUR_AUTH_COOKIE' 'http://localhost:8080/api/v1/admin/sheet-bookings/weekly?start_date=2026-11-02&end_date=2026-11-08&building=ENG%20100'
```
