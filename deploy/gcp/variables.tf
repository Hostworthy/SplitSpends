variable "project_id" {
  type = string
}

variable "region" {
  description = "Cloud Run free tier applies in us-central1, us-east1 and us-west1."
  type        = string
  default     = "us-central1"
}

variable "name" {
  type    = string
  default = "splitspends"
}

variable "image" {
  description = "Container image, for example us-central1-docker.pkg.dev/PROJECT/splitspends/app:v1. Empty on the first apply (creates the registry only)."
  type        = string
  default     = ""
}

variable "database_url" {
  description = "External PostgreSQL URL (Neon or Supabase free tier, sslmode=require). Leave empty and set create_database = true to use Cloud SQL instead."
  type        = string
  default     = ""
  sensitive   = true
}

variable "create_database" {
  description = "Create a small Cloud SQL PostgreSQL instance (billed, roughly 10 USD a month)."
  type        = bool
  default     = false
}

variable "base_url" {
  description = "Public URL if you map a custom domain. Empty uses the Cloud Run URL."
  type        = string
  default     = ""
}

variable "signup_mode" {
  type    = string
  default = "invite"
}

variable "admin_email" {
  description = "Your email address: only it can create the first account, which becomes the administrator. Empty lets whoever signs in first take it."
  type        = string
  default     = ""
}

variable "smtp" {
  type = object({
    host     = string
    port     = optional(number, 587)
    security = optional(string, "starttls")
    username = optional(string, "")
    password = optional(string, "")
    from     = string
  })
  sensitive = true
}

variable "ai" {
  type = object({
    provider = optional(string, "none")
    api_key  = optional(string, "")
    model    = optional(string, "")
    base_url = optional(string, "")
    # Anthropic only; empty uses the model default.
    effort = optional(string, "")
  })
  default   = {}
  sensitive = true
}

variable "max_instances" {
  type    = number
  default = 2
}
