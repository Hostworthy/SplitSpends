variable "project_id" {
  description = "Google Cloud project to deploy into."
  type        = string
}

variable "region" {
  description = "Cloud Run free tier applies in us-central1, us-east1 and us-west1."
  type        = string
  default     = "us-central1"
}

variable "name" {
  description = "Prefix for every resource."
  type        = string
  default     = "splitspends"
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
  description = "Starting sign-up mode: open, invite, approval or closed (administrators can change it in the app)."
  type        = string
  default     = "invite"
  validation {
    # domain is still accepted by the server as invite with signup_allowed_domains.
    condition     = contains(["open", "invite", "approval", "closed", "domain"], var.signup_mode)
    error_message = "signup_mode must be open, invite, approval or closed."
  }
}

variable "admin_email" {
  description = "Your email address: only it can create the first account, which becomes the administrator. Empty lets whoever signs in first take it."
  type        = string
  default     = ""
}

variable "signup_allowed_domains" {
  description = "Comma-separated email domains that start out on the sign-up allowed list, for example example.com. Empty for none."
  type        = string
  default     = ""
}

variable "smtp" {
  description = "SMTP settings for login codes and notifications."
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
  description = "Optional AI provider for receipt scanning."
  type = object({
    provider = optional(string, "none")
    api_key  = optional(string, "")
    model    = optional(string, "")
    base_url = optional(string, "")
    # Anthropic only; empty uses the model default.
    effort = optional(string, "")
    # Requests each person may make per hour (AI_RATE_LIMIT).
    rate_limit = optional(number, 30)
  })
  default   = {}
  sensitive = true
}

variable "max_instances" {
  description = "Most Cloud Run instances at once. Each opens up to 10 database connections (DATABASE_MAX_CONNS), so keep max_instances x 10 below your database's connection limit."
  type        = number
  default     = 2
}

variable "jobs_schedule" {
  description = "How often recurring expenses and cleanup run, as a cron expression in UTC."
  type        = string
  default     = "*/30 * * * *"
}
