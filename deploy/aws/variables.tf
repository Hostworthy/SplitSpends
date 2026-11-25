variable "name" {
  description = "Prefix for every resource."
  type        = string
  default     = "splitspends"
}

variable "region" {
  description = "AWS region."
  type        = string
  default     = "us-east-1"
}

variable "database_url" {
  description = "PostgreSQL URL with sslmode=require, for example a Neon or Supabase free database, or an RDS instance reachable from Lambda."
  type        = string
  sensitive   = true
}

variable "base_url" {
  description = "Public URL if you put a custom domain in front. Empty uses the CloudFront domain."
  type        = string
  default     = ""
}

variable "signup_mode" {
  description = "Starting sign-up mode: open, invite, approval or closed (administrators can change it in the app)."
  type        = string
  default     = "invite"
}

variable "admin_email" {
  description = "Your email address: only it can create the first account, which becomes the administrator. Empty lets whoever signs in first take it."
  type        = string
  default     = ""
}

variable "signup_allowed_domains" {
  type    = string
  default = ""
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
    # Anthropic only. Low keeps receipt scans inside the 30 second gateway limit.
    effort = optional(string, "low")
  })
  default   = {}
  sensitive = true
}

variable "lambda_dir" {
  description = "Directory with the arm64 server binary and run.sh (make lambda)."
  type        = string
  default     = "build/lambda"
}

variable "web_dir" {
  description = "Directory with the Flutter web build (make web-build)."
  type        = string
  default     = "../../apps/web/build/web"
}

variable "lambda_web_adapter_layer" {
  description = "ARN of the AWS Lambda Web Adapter layer for arm64 in your region. See https://github.com/awslabs/aws-lambda-web-adapter."
  type        = string
  default     = ""
}

variable "jobs_schedule" {
  description = "How often recurring expenses and cleanup run."
  type        = string
  default     = "rate(30 minutes)"
}
