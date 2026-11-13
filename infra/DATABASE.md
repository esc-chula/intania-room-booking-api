# Cloud SQL deployment and database operations

This procedure targets the university production project and its existing Terraform state. Cloud SQL is billable as soon as it is created. The instance has a public IP for the IAM-authorized Cloud SQL Auth Proxy; no authorized networks are configured. Confirm your organization permits public IPs. The initial instance is single-zone; a zonal outage requires manual recovery.

## 1. Configure GitHub and identities

Merge the changes to `main` so the workflows are available. Protect `main` and create GitHub environments `infra-apply`, `release`, and `database-production`; restrict all three to `main` and set required reviewers, especially for `database-production`. Confirm your GitHub plan supports required reviewers. Anyone who can edit protected workflow files or environment settings can affect production.

Keep the existing repository variables `GCP_PROJECT_ID`, `GCP_REGION`, `GCP_REGISTRY`, `TF_STATE_BUCKET`, `TF_STATE_PREFIX`, `GOOGLE_SPREADSHEET_ID`, `GOOGLE_SHEET_RANGE`, `GCP_WIF_PROVIDER`, `GCP_PUBLISH_SA`, `GCP_TERRAFORM_PLAN_SA`, and `GCP_TERRAFORM_SA`. Add these repository variables:

| Variable | Value |
| --- | --- |
| `GCP_DATABASE_SA` | `github-database@PROJECT_ID.iam.gserviceaccount.com` |
| `GCP_SECRETS_SA` | `github-secrets@PROJECT_ID.iam.gserviceaccount.com` |
| `CLOUD_SQL_CONNECTION_NAME` | `PROJECT_ID:asia-southeast1:room-booking-postgres` |
| `DB_RUNTIME_ENABLED` | `false` for the first infrastructure apply; later `true` |
| `GOOGLE_CLIENT_ID` | Production Google OAuth web client ID |
| `GOOGLE_REDIRECT_URL` | HTTPS API origin plus `/auth/google/callback`, registered with Google OAuth |
| `FRONTEND_URL` | HTTPS frontend origin |
| `RUNTIME_SECRET_VERSIONS` | After syncing, JSON with the three version numbers, for example `{"database-url":"1","jwt-secret":"1","google-client-secret":"1"}` |

In GitHub environment **infra-apply**, add these environment secrets:

- `RUNTIME_DATABASE_URL`: a pgx PostgreSQL connection string for user `room_app`, database `room_booking`, with host `/cloudsql/PROJECT_ID:asia-southeast1:room-booking-postgres`. For a URL, percent-encode special characters in the password and socket host. Example structure: `postgresql://room_app:PASSWORD@/room_booking?host=%2Fcloudsql%2FPROJECT_ID%3Aasia-southeast1%3Aroom-booking-postgres&sslmode=disable`. The Cloud Run proxy encrypts the network connection.
- `JWT_SECRET`: random secret of at least 32 non-padding bytes.
- `GOOGLE_CLIENT_SECRET`: OAuth web client secret corresponding to the client ID.

In GitHub environment **database-production**, add:

- `DATABASE_ADMIN_URL`: `postgresql://postgres:ENCODED_POSTGRES_PASSWORD@127.0.0.1:5433/room_booking?sslmode=disable`. The workflow opens an authenticated encrypted Cloud SQL Auth Proxy on the GitHub runner's localhost port 5433. Percent-encode special characters in the password. This URL is different from `RUNTIME_DATABASE_URL`, which uses the Cloud Run socket.
- `BOOTSTRAP_ADMIN_EMAIL`: eligible student email ending in `21` for initial administrator creation.
- `OS_STUDENT_IDS`: comma-separated student IDs ending in `21`; update it before running bootstrap OS when membership changes.

Never put these values into Terraform variables, manually typed commands, logs, or repository variables. Goose receives the database URL as an argument on the isolated GitHub runner. Set the `postgres` password in the Cloud SQL console. To create a restricted `room_app` entirely in the browser, open Cloud SQL Studio as `postgres` and run this in the `room_booking` database:

```sql
CREATE ROLE room_app_runtime NOLOGIN;
```

Then, on the instance's Users page, add a built-in user named `room_app` with a separate password. In the Database roles picker, remove `cloudsqlsuperuser` and select `room_app_runtime`. Cloud SQL grants `cloudsqlsuperuser` automatically if a built-in user is created with no custom role. If the custom role is not available in the picker, stop before adding the user. Store both passwords in the organization's approved password manager. If `room_app` already exists, inspect its memberships and privileges before changing it.

Apply the **bootstrap** Terraform stack using its existing local state and correct university project and bucket values. Review its plan: it adds the `github-database` and `github-secrets` service accounts, federation mappings, Cloud SQL Client for the database and API identities, and Cloud SQL/Secret Manager administration for the existing apply identity. The account applying bootstrap must have the necessary IAM permissions. The API's Cloud SQL Client grant lives in bootstrap because the infrastructure workflow cannot edit project IAM. If an earlier infrastructure apply failed at `google_project_iam_member.api_cloudsql`, apply bootstrap again after updating the code, then rerun the infrastructure workflow; keep the existing Terraform state and Cloud SQL instance. Do not create service-account keys. Preserve the existing university state; do not use the personal test example state.

From the repository root, with `TF_VAR_github_repository_id` and `TF_VAR_github_owner_id` set to the existing numeric IDs, review and apply with the university project's actual values:

```sh
terraform -chdir=infra/bootstrap plan -var="project_id=PROJECT_ID" -var="state_bucket_name=STATE_BUCKET" -var="region=asia-southeast1" -out=/tmp/room-booking-bootstrap.tfplan
terraform -chdir=infra/bootstrap apply /tmp/room-booking-bootstrap.tfplan
```

## 2. Create database and secrets

With `DB_RUNTIME_ENABLED=false`, run **Terraform infrastructure**, review the plan, and approve its `infra-apply` job. This creates the database, empty Secret Manager secrets and IAM bindings while preserving the existing Cloud Run image. Do not omit the existing image or state variables. If the project is new, first follow the registry and state bootstrap in `infra/README.md`, then complete the bootstrap identities before this apply.

In Cloud SQL, set the `postgres` user's password, then create `room_app` with a separate password using Cloud SQL Studio and the Users page as described above. Add the GitHub secrets described above, then run **Sync runtime secrets** and approve it. Record the three numeric version IDs from its job summary and set `RUNTIME_SECRET_VERSIONS` to those values. This workflow adds new versions every time it runs; existing Cloud Run revisions remain pinned to their old versions until the infrastructure is applied again.

## 3. Migrate, bootstrap, and grant runtime access

Run **migrate-up**, then **bootstrap-admin** and **bootstrap-os**, approving each `database-production` job. They connect as the postgres administrator using a temporary proxy. Admin bootstrap creates an administrator only when one does not already exist; an empty OS list adds nobody. The workflows use the current `main` commit and are serialized with releases and infrastructure changes. Run **migrate-up** again before releasing API code that uses a newer migration, including the temporary OAuth state table.

In Cloud SQL Studio, connect as `postgres` to `room_booking` and grant the API's login role access through its `room_app_runtime` membership. Run this after both migrations and before enabling Cloud Run. The API currently uses only `users` and `oauth_states`; the bootstrap workflows use the separate `postgres` administrator connection:

```sql
GRANT CONNECT ON DATABASE room_booking TO room_app_runtime;
GRANT USAGE ON SCHEMA public TO room_app_runtime;
GRANT SELECT, INSERT, UPDATE ON TABLE public.users TO room_app_runtime;
GRANT SELECT, INSERT, DELETE ON TABLE public.oauth_states TO room_app_runtime;
```

Check that `pg_has_role('room_app', 'room_app_runtime', 'USAGE')` is true, then verify the effective table privileges with `has_table_privilege`. If a future API feature uses another table, grant only the operations it needs as part of that migration. These grants deliberately include no default access to future tables or sequences. Cloud SQL Auth Proxy connection instructions, if using `psql` instead of Studio: https://docs.cloud.google.com/sql/docs/postgres/connect-auth-proxy

**migrate-down** rolls back the latest migration. The initial migration's down section drops all application tables and data. Inspect the migration and backup first. Its protected workflow additionally requires the exact production project ID as its confirmation input; a GitHub environment reviewer must approve it.

## 4. Enable Cloud Run and release

Set repository variable `DB_RUNTIME_ENABLED=true` and verify all OAuth variables and numeric secret versions. For the first public rollout, stage the change across two commits: keep Cloud Run private in the first commit, run **migrate-up**, apply **Terraform infrastructure** to supply the database and OAuth settings, then run **Release** with that commit's successful main CI SHA. In the second commit, enable public invocation and apply **Terraform infrastructure** again. Review and approve each plan. Release rejects pending infrastructure changes, and Terraform preserves the deployed image. If public invocation has already been applied to an older image whose Sheet routes are unprotected, release the authenticated image immediately or restore private invocation until it is deployed.

Cloud Run permits public invocation so the browser can reach `/auth/google/login` and `/auth/google/callback`. The API still requires its own auth cookie on the three Sheet routes. The named tester and frontend service-account invoker grants remain for a possible private rollback; they do not restrict public requests while the Invoker IAM check is disabled. Verify `GOOGLE_REDIRECT_URL` exactly matches the HTTPS API callback URI registered with Google. The frontend must use that callback's API origin for login, `/api/v1/auth/me`, and the Sheet routes so the host-only auth cookie is sent. The handler redirects login requests from Cloud Run's alternate hostname to the callback host before setting the temporary cookie. When making an existing service public, first migrate and release the authenticated API image while Cloud Run is private, then merge and apply the public Terraform change separately. The Terraform workflow preserves the deployed image, and the Release workflow rejects pending infrastructure changes; applying public access before releasing the authenticated image can expose Sheet data.

## 5. Verify and operate

- Check the Cloud Run revision and call `/health` and `/ready` anonymously. The Terraform workflow checks these public probes, the OAuth login redirect, and the three Sheet routes without cookies after apply; it expects 401 when database runtime is enabled. A failed post-apply check does not roll back the apply. After Release, verify each Sheet route returns 401 without a cookie, then check OAuth login, `/api/v1/auth/me`, and logout through the intended frontend. A successful `/ready` verifies the configured database ping, not OAuth.
- In Cloud SQL, confirm successful automated backups and point-in-time recovery. Restore a backup to a **new temporary instance**, validate the schema and users, then delete only that temporary instance. See https://docs.cloud.google.com/sql/docs/postgres/backup-recovery/restore
- Set billing and database resource alerts, and assign two university operators access to the project and recovery documentation. Budget alerts do not stop charges.
- For secret rotation, update the `infra-apply` GitHub secrets, run **Sync runtime secrets**, update all three version IDs in `RUNTIME_SECRET_VERSIONS`, and apply infrastructure to create a new Cloud Run revision. Coordinate database password changes with the deployment; changing a user's password before the new revision is live can interrupt connections. JWT rotation invalidates existing cookies.
