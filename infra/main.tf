resource "google_project_service" "required" {
  for_each = toset([
    "artifactregistry.googleapis.com",
    "iam.googleapis.com",
    "run.googleapis.com",
    "serviceusage.googleapis.com",
    "sheets.googleapis.com",
  ])

  project            = var.project_id
  service            = each.value
  disable_on_destroy = false
}
resource "google_artifact_registry_repository" "api" {
  location      = var.region
  repository_id = "room-booking"
  description   = "Room booking API container images"
  format        = "DOCKER"
  depends_on    = [google_project_service.required]
}
resource "google_service_account" "api" {
  account_id   = "room-booking-api"
  display_name = "Room booking API runtime"
  depends_on   = [google_project_service.required]
}
resource "google_service_account" "frontend" {
  account_id   = "nextjs-frontend"
  display_name = "Next.js frontend runtime"
  depends_on   = [google_project_service.required]
}
resource "google_cloud_run_v2_service" "api" {
  count                = var.image_url == "" ? 0 : 1
  name                 = "room-booking-api"
  location             = var.region
  ingress              = "INGRESS_TRAFFIC_ALL"
  invoker_iam_disabled = false
  deletion_protection  = false

  scaling {
    min_instance_count = 0
    max_instance_count = 1
  }

  template {
    service_account = google_service_account.api.email

    containers {
      image = var.image_url

      ports {
        container_port = 8080
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "512Mi"
        }
        cpu_idle = true
      }

      env {
        name  = "GOOGLE_SPREADSHEET_ID"
        value = var.spreadsheet_id
      }
      env {
        name  = "GOOGLE_SHEET_RANGE"
        value = var.sheet_range
      }
    }
  }

  depends_on = [google_project_service.required]
}

# Allow the org test account to invoke the private API.
resource "google_cloud_run_v2_service_iam_member" "tester" {
  count    = var.image_url == "" ? 0 : 1
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.api[0].name
  role     = "roles/run.invoker"
  member   = "user:tech@intania.org"
}

resource "google_cloud_run_v2_service_iam_member" "frontend" {
  count    = var.image_url == "" ? 0 : 1
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.api[0].name
  role     = "roles/run.invoker"
  member   = google_service_account.frontend.member
}
