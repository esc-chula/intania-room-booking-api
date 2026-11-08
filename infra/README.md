# Personal GCP test

This stack deploys the existing read-only Sheets API to Cloud Run. The API is
public and can return requester names and booking details. Use this only with
data you are authorized to expose. It does not implement authentication,
booking writes, approvals, conflict handling, or the rest of the EPIC.

The examples explicitly target `intania-room-booking-test`. Do not reuse its
state for the organization; inspect that project and agree on its own state and
access policies first. Do not commit credentials, state, private variable files,
or plans. APIs are intentionally left enabled after destroy.

## Prerequisites and inventory

Install Terraform, Google Cloud CLI and Docker with a running daemon. Sign in
with `gcloud auth login` and set up Terraform ADC with
`gcloud auth application-default login`. Use an account allowed to inspect and
manage the selected project, enable APIs, create the listed resources, set
Cloud Run IAM, act as the runtime account, and push images. No service-account
key is needed. Set a billing alert manually in the Google Cloud billing console;
an alerts-only budget is not a spending cap.

From the repository root, use a POSIX shell:

```sh
export PROJECT_ID=intania-room-booking-test
export REGION=asia-southeast1
export STATE_BUCKET="${PROJECT_ID}-terraform-state"
gcloud projects describe "$PROJECT_ID"
gcloud billing projects describe "$PROJECT_ID"
gcloud services list --enabled --project "$PROJECT_ID"
gcloud artifacts repositories list --location "$REGION" --project "$PROJECT_ID"
gcloud run services list --region "$REGION" --project "$PROJECT_ID"
gcloud iam service-accounts list --project "$PROJECT_ID"
gcloud storage buckets describe "gs://$STATE_BUCKET" --project "$PROJECT_ID"
```

The bootstrap also requires the numeric GitHub repository and owner IDs. For a
private repository, use an authenticated GitHub CLI session to get them:

```sh
gh api repos/MadManJJ/room-reservation-api --jq '"repository ID: \(.id)\nowner ID: \(.owner.id)"'
export TF_VAR_github_repository_id=REPLACE_WITH_REPOSITORY_ID
export TF_VAR_github_owner_id=REPLACE_WITH_OWNER_ID
```

These environment variables must be set for every bootstrap plan, apply, or
destroy. Use the actual IDs, not the names or placeholders. Restrict the GitHub
environments before applying the identity setup described below.

The last command may report that the bucket does not exist; bucket names are
globally unique, so choose another name if it belongs to someone else. Check
organization policies for public `allUsers` invocation and service-account
creation. Stop if the project or existing resources are not the intended ones.
This test creates `room-booking` (repository) and `room-booking-api` (service
account and service); do not overwrite existing resources. Existing APIs may
already be enabled. If Service Usage is disabled, an authorized administrator
must enable it first: `gcloud services enable serviceusage.googleapis.com
--project "$PROJECT_ID"`. The Cloud Storage API must also be available for the
state bucket. If a policy blocks public invocation, stop and resolve it rather
than bypassing it.

## Bootstrap and deploy

1. Create the GCS backend with its own **local** state. Keep and back up
   `infra/bootstrap/terraform.tfstate` securely until the bucket is destroyed.
   The full bootstrap also creates GitHub federation and IAM bindings, which
   require the registry and runtime account. On a fresh project, create only
   the bucket at this step; complete the identity setup after step 2. The
   targeted apply is only for breaking this first-time dependency cycle.
   The state bucket is versioned and private; its contents may still contain
   sensitive infrastructure details. Soft delete is disabled for this temporary
   bucket so the requested cleanup is complete; removed object versions cannot
   be recovered. Do not use this setting for organization state without review.

   ```sh
   terraform -chdir=infra/bootstrap init
   terraform -chdir=infra/bootstrap plan -target=google_storage_bucket.terraform_state -var="project_id=$PROJECT_ID" -var="state_bucket_name=$STATE_BUCKET" -var="region=$REGION"
   terraform -chdir=infra/bootstrap apply -target=google_storage_bucket.terraform_state -var="project_id=$PROJECT_ID" -var="state_bucket_name=$STATE_BUCKET" -var="region=$REGION"
   ```

2. Initialize the application stack with a separate GCS state prefix and
   create the APIs, registry, and runtime identity. Review each plan before
   applying. The example variables contain the agreed public test Sheet.

   ```sh
   cp infra/terraform.tfvars.example infra/terraform.tfvars
   terraform -chdir=infra init -backend-config="bucket=$STATE_BUCKET" -backend-config="prefix=room-booking/personal-test"
   terraform -chdir=infra plan
   terraform -chdir=infra apply
   terraform -chdir=infra output runtime_service_account
   ```

   If necessary, share the Sheet with the printed service-account email as a
   Viewer. Wait for permissions to propagate. Cloud Run uses ADC from its
   attached identity; it needs no project-wide role to read this Sheet.

   Once the registry and runtime identity exist, configure GitHub environment
   protections as described below, then review and apply the full bootstrap:

   ```sh
   terraform -chdir=infra/bootstrap plan -var="project_id=$PROJECT_ID" -var="state_bucket_name=$STATE_BUCKET" -var="region=$REGION"
   terraform -chdir=infra/bootstrap apply -var="project_id=$PROJECT_ID" -var="state_bucket_name=$STATE_BUCKET" -var="region=$REGION"
   terraform -chdir=infra/bootstrap output github_wif_provider
   terraform -chdir=infra/bootstrap output github_service_accounts
   ```

3. Build for Cloud Run's Linux x86-64 platform and push a versioned image.
   Docker authentication may update your local Docker configuration.

   ```sh
   export REPOSITORY="${REGION}-docker.pkg.dev/${PROJECT_ID}/room-booking"
   export IMAGE_TAG="${REPOSITORY}/api:test-$(git rev-parse --short HEAD)"
   gcloud auth configure-docker "${REGION}-docker.pkg.dev"
   docker build --platform linux/amd64 -t "$IMAGE_TAG" .
   docker push "$IMAGE_TAG"
   export IMAGE_URL="$(docker image inspect --format='{{index .RepoDigests 0}}' "$IMAGE_TAG")"
   printf 'Image digest: %s\n' "$IMAGE_URL"
   ```

   Verify `IMAGE_URL` is a nonempty repository digest. If Docker does not report
   one, obtain the digest from Artifact Registry before continuing. Keep the
   digest available for subsequent plans and destruction.

4. Deploy the image and public invocation binding:

   ```sh
   terraform -chdir=infra plan -var="image_url=$IMAGE_URL"
   terraform -chdir=infra apply -var="image_url=$IMAGE_URL"
   export SERVICE_URL="$(terraform -chdir=infra output -raw service_url)"
   curl -fsS "$SERVICE_URL/health"
   curl -fsS "$SERVICE_URL/ready"
   curl -fsS "$SERVICE_URL/api/v1/admin/sheet-bookings"
   gcloud run services describe room-booking-api --region "$REGION" --project "$PROJECT_ID"
   gcloud run services get-iam-policy room-booking-api --region "$REGION" --project "$PROJECT_ID"
   gcloud logging read 'resource.type="cloud_run_revision" AND resource.labels.service_name="room-booking-api"' --project "$PROJECT_ID" --limit 20
   terraform -chdir=infra plan -var="image_url=$IMAGE_URL"
   ```

   The final plan should show no unexpected changes. The first request may be
   slow due to scale-to-zero; the per-instance Sheets cache expires after 60
   seconds. A maximum of one instance is not a strict cap during replacements
   or traffic spikes. Always pass `image_url` on subsequent plans and applies:
   omitting it selects the bootstrap stage and plans to delete the service.

## Destroy the test

Run this only for the personal test after reviewing the destroy plan. Destroy
the bootstrap IAM and identity resources before deleting the registry and
runtime service account, and preserve the state bucket until the main stack is
destroyed. Disable GitHub workflows first. Review targeted destruction carefully:

```sh
terraform -chdir=infra/bootstrap destroy -target=google_iam_workload_identity_pool.github -target=google_service_account.github -target=google_artifact_registry_repository_iam_member.publisher -target=google_artifact_registry_repository_iam_member.reader -target=google_storage_bucket_iam_member.state -target=google_project_iam_member.plan_viewer -target=google_project_iam_member.apply -target=google_service_account_iam_member.runtime_user -var="project_id=$PROJECT_ID" -var="state_bucket_name=$STATE_BUCKET" -var="region=$REGION"
```

Then destroy the main stack. It deletes
the Cloud Run service, its public binding, the runtime identity, the repository,
and its pushed images. APIs remain enabled. Keep the bootstrap local state until
the final step succeeds.

```sh
terraform -chdir=infra plan -destroy -var="image_url=$IMAGE_URL"
terraform -chdir=infra destroy -var="image_url=$IMAGE_URL"
```

Verify the application resources are gone. The next command permanently deletes
**all objects and all versions** in the state bucket, including recovery copies.
Confirm the bucket name and contents first, and do not run it on a shared bucket.

```sh
gcloud storage ls --all-versions "gs://$STATE_BUCKET/**"
gcloud storage rm --recursive --all-versions "gs://$STATE_BUCKET/**"
terraform -chdir=infra/bootstrap plan -destroy -var="project_id=$PROJECT_ID" -var="state_bucket_name=$STATE_BUCKET" -var="region=$REGION"
terraform -chdir=infra/bootstrap destroy -var="project_id=$PROJECT_ID" -var="state_bucket_name=$STATE_BUCKET" -var="region=$REGION"
```

If the bucket is already empty, skip the `gcloud storage rm` command. After
confirming deletion, remove local Terraform state, plans, and variable files if
no longer needed. Never delete state before its corresponding resources have
been destroyed or transferred to another owner.

## GitHub Actions setup

The three workflows use the existing personal GCS state. Before enabling them,
confirm the registry and runtime account exist, and inventory existing GCP
workload identity pools, service accounts and GitHub environment settings. The
workflows do not create their own credentials or the state bucket. Protect
`main` and changes to workflow files. Do not use service-account keys.

Configure repository variables:

| Variable | Personal test value |
| --- | --- |
| `GCP_PROJECT_ID` | `intania-room-booking-test` |
| `GCP_REGION` | `asia-southeast1` |
| `GCP_REGISTRY` | `room-booking` |
| `TF_STATE_BUCKET` | The bucket created by bootstrap |
| `TF_STATE_PREFIX` | `room-booking/personal-test` (or your actual existing prefix) |
| `GOOGLE_SPREADSHEET_ID` | Your test spreadsheet ID |
| `GOOGLE_SHEET_RANGE` | `จองห้อง!A9:AM` |
| `GCP_WIF_PROVIDER` | Full workload identity provider resource name |
| `GCP_PUBLISH_SA` | Publisher service account email |
| `GCP_TERRAFORM_PLAN_SA` | Terraform plan service account email |
| `GCP_TERRAFORM_SA` | Terraform apply/release service account email |

Set up GitHub OIDC federation constrained to the numeric repository and owner
IDs, `refs/heads/main`, and the relevant workflow/environment claims. The
publisher needs Artifact Registry Writer on this repository only. The plan
identity needs resource read permissions and state-bucket access (including
lock object writes); do not grant it resource modification permissions. The
apply identity needs the permissions to manage the Terraform resources, act as
the runtime identity, read the registry and access state. Restrict its
impersonation to `personal-infra-apply` and `personal-release` environments,
not to all main-branch workflow tokens. Grant only the needed roles after
inventorying existing IAM. Plan artifacts and Terraform state can contain
sensitive data: restrict repository and artifact access and use one-day artifact
retention. Do not put credentials in Terraform variables or GitHub variables.

Create GitHub environments `personal-infra-apply` and `personal-release`,
restrict them to `main`, and configure required reviewers for the infrastructure
apply environment. Confirm your GitHub repository plan supports required
reviewers; do not run the infrastructure workflow without that protection.
Configure the release environment according to the team's approval policy.
The identity provider accepts only these exact workflow files on main and the
specified environment names. A repository administrator can change environment
protections or workflow files, so restrict administrator access and protect
changes to `main`. Both Terraform identities can modify objects in the state
bucket to use locking; treat the plan identity as sensitive too.

CI runs checks on PRs and main pushes; only successful main pushes publish an
image tagged by commit SHA. Start **Release** on `main` with the full SHA of a
successful push to deploy its registry digest. Release refuses unrelated
Terraform changes; apply those using the infrastructure workflow first. The
first release creates Cloud Run. The release checks `/ready` after applying;
a readiness failure is reported but does not automatically roll back. To roll
back, run Release with a previous successful SHA whose image is still present.

Start **Terraform infrastructure** on `main`, inspect its plan in the job
summary, then approve the protected apply job. It preserves the image in state,
including an empty image before the first release. A stale saved plan fails; run
the workflow again and review the new plan. These workflows share a concurrency
group, but local Terraform commands can still overlap: use the same GCS state
and never disable locking. Cloud Run remains public and Sheets data may be
exposed as described above. Test each workflow in the personal project before
adapting settings and separate state to the organization project.
