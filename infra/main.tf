resource "google_project_service" "required" {
  for_each = toset([
    "artifactregistry.googleapis.com",
    "iam.googleapis.com",
    "run.googleapis.com",
    "serviceusage.googleapis.com",
    "sheets.googleapis.com",
    "sqladmin.googleapis.com",
    "secretmanager.googleapis.com",
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

resource "google_sql_database_instance" "booking" {
  name                = "room-booking-postgres"
  region              = var.region
  database_version    = "POSTGRES_18"
  deletion_protection = true
  settings {
    edition                     = "ENTERPRISE"
    tier                        = "db-g1-small"
    availability_type           = "ZONAL"
    disk_type                   = "PD_SSD"
    disk_size                   = 10
    disk_autoresize             = true
    deletion_protection_enabled = true
    ip_configuration {
      ipv4_enabled = true
    }
    backup_configuration {
      enabled                        = true
      point_in_time_recovery_enabled = true
      transaction_log_retention_days = 3
      backup_retention_settings {
        retained_backups = 14
        retention_unit   = "COUNT"
      }
    }
  }
  depends_on = [google_project_service.required]
}

resource "google_sql_database" "booking" {
  name     = "room_booking"
  instance = google_sql_database_instance.booking.name
}

resource "google_secret_manager_secret" "runtime" {
  for_each  = toset(["database-url", "jwt-secret", "google-client-secret"])
  secret_id = "room-booking-${each.key}"
  replication {
    auto {}
  }
  depends_on = [google_project_service.required]
}

resource "google_secret_manager_secret_iam_member" "runtime" {
  for_each  = google_secret_manager_secret.runtime
  secret_id = each.value.id
  role      = "roles/secretmanager.secretAccessor"
  member    = google_service_account.api.member
}

resource "google_secret_manager_secret_iam_member" "writer" {
  for_each  = google_secret_manager_secret.runtime
  secret_id = each.value.id
  role      = "roles/secretmanager.secretVersionAdder"
  member    = "serviceAccount:github-secrets@${var.project_id}.iam.gserviceaccount.com"
}
resource "google_cloud_run_v2_service" "api" {
  count                = var.image_url == "" ? 0 : 1
  name                 = "room-booking-api"
  location             = var.region
  ingress              = "INGRESS_TRAFFIC_ALL"
  invoker_iam_disabled = true
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
      env {
        name  = "APP_ENV"
        value = "production"
      }
      dynamic "env" {
        for_each = var.db_runtime_enabled ? {
          DB_MAX_CONNS     = "5", DB_MIN_CONNS = "0",
          GOOGLE_CLIENT_ID = var.google_client_id, GOOGLE_REDIRECT_URL = var.google_redirect_url,
          FRONTEND_URL     = var.frontend_url, AUTH_COOKIE_SAME_SITE = "none"
        } : {}
        content {
          name  = env.key
          value = env.value
        }
      }
      dynamic "env" {
        for_each = var.db_runtime_enabled ? {
          DATABASE_URL = "database-url", JWT_SECRET = "jwt-secret", GOOGLE_CLIENT_SECRET = "google-client-secret"
        } : {}
        content {
          name = env.key
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.runtime[env.value].secret_id
              version = var.secret_versions[env.value]
            }
          }
        }
      }
      dynamic "volume_mounts" {
        for_each = var.db_runtime_enabled ? [1] : []
        content {
          name       = "cloudsql"
          mount_path = "/cloudsql"
        }
      }
    }
    dynamic "volumes" {
      for_each = var.db_runtime_enabled ? [1] : []
      content {
        name = "cloudsql"
        cloud_sql_instance { instances = [google_sql_database_instance.booking.connection_name] }
      }
    }
  }

  lifecycle {
    precondition {
      condition     = !var.db_runtime_enabled || (var.google_client_id != "" && var.google_redirect_url != "" && var.frontend_url != "")
      error_message = "Configure OAuth and frontend URLs before enabling database runtime."
    }
  }
  depends_on = [google_project_service.required, google_secret_manager_secret_iam_member.runtime]
}

# Retain these invoker grants if the API is made private again.
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
