variable "github_repository_id" {
  description = "Numeric GitHub repository ID."
  type        = string
  validation {
    condition     = can(regex("^[0-9]+$", var.github_repository_id))
    error_message = "github_repository_id must be a numeric GitHub repository ID."
  }
}

variable "github_owner_id" {
  description = "Numeric GitHub repository owner ID."
  type        = string
  validation {
    condition     = can(regex("^[0-9]+$", var.github_owner_id))
    error_message = "github_owner_id must be a numeric GitHub owner ID."
  }
}

variable "github_repository" {
  description = "GitHub owner/repository, used to restrict the exact workflow files."
  type        = string
  default     = "esc-chula/intania-room-booking-api"
  validation {
    condition     = can(regex("^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$", var.github_repository))
    error_message = "github_repository must be an owner/repository name."
  }
}

data "google_project" "current" {
  project_id = var.project_id
}

resource "google_project_service" "federation" {
  for_each = toset([
    "iamcredentials.googleapis.com",
    "sts.googleapis.com",
    "cloudresourcemanager.googleapis.com",
  ])
  project            = var.project_id
  service            = each.value
  disable_on_destroy = false
}

resource "google_iam_workload_identity_pool" "github" {
  workload_identity_pool_id = "github-actions"
  display_name              = "GitHub Actions"
  depends_on                = [google_project_service.federation]
}

resource "google_iam_workload_identity_pool_provider" "github" {
  workload_identity_pool_id          = google_iam_workload_identity_pool.github.workload_identity_pool_id
  workload_identity_pool_provider_id = "github"
  display_name                       = "GitHub Actions"
  attribute_condition                = "assertion.repository_id == '${var.github_repository_id}' && assertion.repository_owner_id == '${var.github_owner_id}' && assertion.ref == 'refs/heads/main'"
  attribute_mapping = {
    "google.subject"   = "assertion.sub"
    "attribute.access" = "assertion.workflow_ref == '${var.github_repository}/.github/workflows/ci.yml@refs/heads/main' ? 'publisher' : assertion.workflow_ref == '${var.github_repository}/.github/workflows/terraform.yml@refs/heads/main' && !has(assertion.environment) ? 'plan' : assertion.workflow_ref == '${var.github_repository}/.github/workflows/terraform.yml@refs/heads/main' && has(assertion.environment) && assertion.environment == 'infra-apply' ? 'apply' : assertion.workflow_ref == '${var.github_repository}/.github/workflows/release.yml@refs/heads/main' && has(assertion.environment) && assertion.environment == 'release' ? 'apply' : assertion.workflow_ref == '${var.github_repository}/.github/workflows/sync-secrets.yml@refs/heads/main' && has(assertion.environment) && assertion.environment == 'infra-apply' ? 'secrets' : assertion.workflow_ref in ['${var.github_repository}/.github/workflows/migrate-up.yml@refs/heads/main', '${var.github_repository}/.github/workflows/migrate-down.yml@refs/heads/main', '${var.github_repository}/.github/workflows/bootstrap-os.yml@refs/heads/main', '${var.github_repository}/.github/workflows/bootstrap-admin.yml@refs/heads/main'] && has(assertion.environment) && assertion.environment == 'database-production' ? 'database' : 'denied'"
  }
  oidc {
    issuer_uri = "https://token.actions.githubusercontent.com"
  }
}

resource "google_service_account" "github" {
  for_each     = toset(["publisher", "plan", "apply", "database", "secrets"])
  account_id   = "github-${each.key}"
  display_name = "GitHub Actions ${each.key}"
}

resource "google_service_account_iam_member" "federation" {
  for_each           = google_service_account.github
  service_account_id = each.value.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.github.name}/attribute.access/${each.key}"
  depends_on         = [google_iam_workload_identity_pool_provider.github]
}

resource "google_artifact_registry_repository_iam_member" "publisher" {
  project    = var.project_id
  location   = var.region
  repository = "room-booking"
  role       = "roles/artifactregistry.writer"
  member     = google_service_account.github["publisher"].member
}

resource "google_artifact_registry_repository_iam_member" "reader" {
  project    = var.project_id
  location   = var.region
  repository = "room-booking"
  role       = "roles/artifactregistry.reader"
  member     = google_service_account.github["apply"].member
}

resource "google_storage_bucket_iam_member" "state" {
  for_each = toset(["plan", "apply"])
  bucket   = google_storage_bucket.terraform_state.name
  role     = "roles/storage.objectAdmin"
  member   = google_service_account.github[each.key].member
}

resource "google_project_iam_member" "plan_viewer" {
  for_each = toset(["roles/viewer", "roles/iam.securityReviewer"])
  project  = var.project_id
  role     = each.value
  member   = google_service_account.github["plan"].member
}

resource "google_project_iam_member" "apply" {
  for_each = toset([
    "roles/viewer",
    "roles/run.admin",
    "roles/artifactregistry.admin",
    "roles/iam.serviceAccountAdmin",
    "roles/serviceusage.serviceUsageAdmin",
    "roles/cloudsql.admin",
    "roles/secretmanager.admin",
  ])
  project = var.project_id
  role    = each.value
  member  = google_service_account.github["apply"].member
}

resource "google_project_iam_member" "database_client" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = google_service_account.github["database"].member
}

resource "google_service_account_iam_member" "runtime_user" {
  service_account_id = "projects/${var.project_id}/serviceAccounts/room-booking-api@${var.project_id}.iam.gserviceaccount.com"
  role               = "roles/iam.serviceAccountUser"
  member             = google_service_account.github["apply"].member
}

output "github_wif_provider" {
  value = "projects/${data.google_project.current.number}/locations/global/workloadIdentityPools/${google_iam_workload_identity_pool.github.workload_identity_pool_id}/providers/${google_iam_workload_identity_pool_provider.github.workload_identity_pool_provider_id}"
}

output "github_service_accounts" {
  value = { for key, account in google_service_account.github : key => account.email }
}
