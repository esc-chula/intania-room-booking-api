# Chula Room Booking API

This API reads legacy room bookings from Google Sheets. The three endpoints below are public, read-only, and available without an API login or PostgreSQL. Despite the `/admin` path segment, they do not require an OS or ADMIN account.

## Local development

You need Go 1.26.8, Make, the Google Cloud CLI, a Google Cloud project with the Google Sheets API enabled, and a spreadsheet that the service account can view.

1. Sign in to `gcloud`, then create local Application Default Credentials (ADC) by impersonating the service account. Your Google account needs permission to impersonate it (for example, `roles/iam.serviceAccountTokenCreator`), and the spreadsheet must be shared with the service account as a Viewer.

   ```sh
   gcloud auth login
   gcloud auth application-default login --impersonate-service-account=YOUR_SERVICE_ACCOUNT@YOUR_PROJECT.iam.gserviceaccount.com
   ```

   `gcloud auth login` alone does not provide credentials to the Go API. It uses ADC to call Sheets with read-only access. See [Google's ADC documentation](https://docs.cloud.google.com/docs/authentication/application-default-credentials). If `GOOGLE_APPLICATION_CREDENTIALS` is set in your shell, ADC uses that file first; unset it to use the impersonated credentials above.

2. Copy the environment file and set the spreadsheet ID and range in `.env`:

   ```sh
   cp .env.example .env
   ```

   ```sh
   GOOGLE_SPREADSHEET_ID=your-spreadsheet-id
   GOOGLE_SHEET_RANGE='จองห้อง!A9:AM'
   ```

   The spreadsheet ID is between `/d/` and `/edit` in its URL. Set the range to the correct tab and data rows; it must start at column A and include column AE. Keep quotes around a range containing spaces or shell-sensitive characters. Leave `DATABASE_URL`, `JWT_SECRET`, and the Google OAuth client variables empty for this setup.

3. Start the API:

   ```sh
   make run
   ```

   `make run` loads `.env`. If you use `go run ./cmd/api` directly, export those variables yourself. The default address is `http://localhost:8080`.

## Google Sheets booking endpoints

All three endpoints return JSON, use the same 60-second in-memory Sheet cache, and set `Cache-Control: no-store`. Invalid query parameters return `400` with `{"error":"invalid query parameters"}`. A Sheets fetch failure returns `502` with `{"error":"failed to fetch sheet bookings"}`.

If you get `502` locally, check that the Sheets API is enabled for your project, the service account can view the spreadsheet, and the ID and range in `.env` are correct.

### List bookings

`GET /api/v1/admin/sheet-bookings`

| Query parameter | Behavior |
| --- | --- |
| `page`, `page_size` | Defaults: 1 and 20. Maximum page size: 100. |
| `year`, `month` | Optional, but must be supplied together. Includes bookings whose date range overlaps the month. |
| `building` | Optional: `ENG 3`, `ENG 100`, or `ENG 4`. |
| `status` | Optional: `approved`, `cancelled`, `submitted`, `timeline`, `needs_revision`, or `conflicted`. |

The response is `{"items":[...],"page":1,"page_size":20,"total":0}`. Each item has `id`, `name`, `room`, `start_date`, `end_date`, `start_time`, `end_time`, `status`, and `conflicted`. The `conflicted` filter selects active bookings that overlap another active booking in the same room and time window.

```sh
curl 'http://localhost:8080/api/v1/admin/sheet-bookings?year=2026&month=11&page=1&page_size=20'
```

### Monthly calendar

`GET /api/v1/admin/sheet-bookings/calendar`

`year` and `month` are required. `building` is optional and accepts the same three values as the list endpoint. The response has `year`, `month`, and `days`. Each day contains a `date` in `YYYY-MM-DD` format and `counts` with `approved`, `cancelled`, `submitted`, `timeline`, `needs_revision`, and `conflicted`. Days with no bookings are omitted. Conflict counts are additional to the status counts.

```sh
curl 'http://localhost:8080/api/v1/admin/sheet-bookings/calendar?year=2026&month=11&building=ENG%203'
```

### Weekly room schedule

`GET /api/v1/admin/sheet-bookings/weekly`

| Query parameter | Behavior |
| --- | --- |
| `start_date` | Optional `YYYY-MM-DD`; defaults to today in Bangkok. |
| `end_date` | Optional inclusive `YYYY-MM-DD`; defaults to six days after `start_date`. Requires `start_date` if supplied. The range may span at most seven days. |
| `building` | Optional: `ENG 3`, `ENG 100`, or `ENG 4`. |

The response has `start_date`, `end_date`, and `rooms`. Every catalog room in the selected buildings appears, including rooms with an empty `bookings` array. Each room has `id`, `building`, `name`, and `bookings`. Each booking has `id`, `title`, `start_at`, `end_at`, `status`, and `has_conflict`; timestamps use Bangkok time. Multi-day bookings appear on each day they cover.

```sh
curl 'http://localhost:8080/api/v1/admin/sheet-bookings/weekly?start_date=2026-11-02&end_date=2026-11-08&building=ENG%20100'
```
