output "repository_url" {
  value = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.api.repository_id}"
}

output "runtime_service_account" {
  value = google_service_account.api.email
}

output "frontend_service_account" {
  value = google_service_account.frontend.email
}

output "service_url" {
  value = try(google_cloud_run_v2_service.api[0].uri, null)
}

output "cloud_sql_connection_name" {
  value = google_sql_database_instance.booking.connection_name
}
