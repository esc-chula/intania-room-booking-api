variable "project_id" {
  type = string
}

variable "region" {
  type    = string
  default = "asia-southeast1"
}

variable "spreadsheet_id" {
  type = string
}

variable "sheet_range" {
  type = string
}

variable "image_url" {
  description = "Artifact Registry image URL; leave empty for the initial registry and identity bootstrap."
  type        = string
  default     = ""
}

variable "db_runtime_enabled" {
  type    = bool
  default = false
}
variable "google_client_id" {
  type    = string
  default = ""
}
variable "google_redirect_url" {
  type    = string
  default = ""
}
variable "frontend_url" {
  type    = string
  default = ""
}
variable "secret_versions" {
  type = map(string)
  default = {
    database-url         = "1"
    jwt-secret           = "1"
    google-client-secret = "1"
  }
}
