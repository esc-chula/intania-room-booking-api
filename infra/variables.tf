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
