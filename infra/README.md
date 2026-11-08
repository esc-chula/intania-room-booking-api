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
   The state bucket is versioned and private; its contents may still contain
   sensitive infrastructure details. Soft delete is disabled for this temporary
   bucket so the requested cleanup is complete; removed object versions cannot
   be recovered. Do not use this setting for organization state without review.

   ```sh
   terraform -chdir=infra/bootstrap init
   terraform -chdir=infra/bootstrap plan -var="project_id=$PROJECT_ID" -var="state_bucket_name=$STATE_BUCKET" -var="region=$REGION"
   terraform -chdir=infra/bootstrap apply -var="project_id=$PROJECT_ID" -var="state_bucket_name=$STATE_BUCKET" -var="region=$REGION"
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
   curl -fsS "$SERVICE_URL/healthz"
   curl -fsS "$SERVICE_URL/readyz"
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

Run this only for the personal test after reviewing the destroy plan. It deletes
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
