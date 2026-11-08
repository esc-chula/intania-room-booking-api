terraform {
  required_version = ">= 1.5.0, < 2.0.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 7.0"
    }
  }
}

variable "project_id" {
  type = string
}

variable "state_bucket_name" {
  type = string
}

variable "region" {
  type    = string
  default = "asia-southeast1"
}

provider "google" {
  project = var.project_id
  region  = var.region
}

# This stack deliberately uses local state: a backend cannot manage itself.
resource "google_storage_bucket" "terraform_state" {
  name                        = var.state_bucket_name
  location                    = var.region
  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"
  force_destroy               = false

  versioning {
    enabled = true
  }

  # The personal test explicitly requires complete cleanup after destruction.
  soft_delete_policy {
    retention_duration_seconds = 0
  }
}

output "state_bucket_name" {
  value = google_storage_bucket.terraform_state.name
}
