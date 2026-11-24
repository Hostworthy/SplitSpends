data "google_project" "this" {}

locals {
  services = ["run.googleapis.com", "secretmanager.googleapis.com", "cloudscheduler.googleapis.com",
  "artifactregistry.googleapis.com", "sqladmin.googleapis.com", "iam.googleapis.com"]
  # Cloud Run's deterministic URL, known before the service exists.
  run_url  = "https://${var.name}-${data.google_project.this.number}.${var.region}.run.app"
  base_url = var.base_url != "" ? trimsuffix(var.base_url, "/") : local.run_url
  database_url = var.create_database ? (
    "postgres://splitspends:${random_password.db[0].result}@/splitspends?host=/cloudsql/${google_sql_database_instance.db[0].connection_name}"
  ) : var.database_url
  deploy = var.image != ""
}

resource "google_project_service" "apis" {
  for_each           = toset(local.services)
  service            = each.value
  disable_on_destroy = false
}

resource "google_artifact_registry_repository" "images" {
  repository_id = var.name
  location      = var.region
  format        = "DOCKER"
  depends_on    = [google_project_service.apis]
}

resource "random_password" "secret_key" {
  length  = 64
  special = false
}

resource "random_password" "jobs_secret" {
  length  = 48
  special = false
}

# ---------------------------------------------------------------- database

resource "random_password" "db" {
  count   = var.create_database ? 1 : 0
  length  = 32
  special = false
}

resource "google_sql_database_instance" "db" {
  count               = var.create_database ? 1 : 0
  name                = "${var.name}-db"
  database_version    = "POSTGRES_16"
  deletion_protection = true
  settings {
    tier              = "db-f1-micro"
    edition           = "ENTERPRISE"
    availability_type = "ZONAL"
    disk_size         = 10
    backup_configuration {
      enabled                        = true
      point_in_time_recovery_enabled = true
    }
    ip_configuration {
      # Public IP with no authorized networks: only the Cloud SQL connector
      # (IAM-authenticated, used by Cloud Run) can connect.
      ipv4_enabled = true
      ssl_mode     = "ENCRYPTED_ONLY"
    }
  }
  depends_on = [google_project_service.apis]
}

resource "google_sql_database" "db" {
  count    = var.create_database ? 1 : 0
  name     = "splitspends"
  instance = google_sql_database_instance.db[0].name
}

resource "google_sql_user" "db" {
  count    = var.create_database ? 1 : 0
  name     = "splitspends"
  instance = google_sql_database_instance.db[0].name
  password = random_password.db[0].result
}

# ---------------------------------------------------------------- secrets

locals {
  secrets = {
    DATABASE_URL  = local.database_url
    SECRET_KEY    = random_password.secret_key.result
    JOBS_SECRET   = random_password.jobs_secret.result
    SMTP_PASSWORD = var.smtp.password
    AI_API_KEY    = var.ai.api_key
  }
}

resource "google_secret_manager_secret" "app" {
  for_each  = nonsensitive(toset(keys(local.secrets)))
  secret_id = "${var.name}-${lower(replace(each.value, "_", "-"))}"
  replication {
    auto {}
  }
  depends_on = [google_project_service.apis]
}

resource "google_secret_manager_secret_version" "app" {
  for_each    = google_secret_manager_secret.app
  secret      = each.value.id
  secret_data = local.secrets[each.key] != "" ? local.secrets[each.key] : "unset"
}

resource "google_service_account" "run" {
  account_id   = "${var.name}-run"
  display_name = "Split Spends Cloud Run"
}

resource "google_secret_manager_secret_iam_member" "run" {
  for_each  = google_secret_manager_secret.app
  secret_id = each.value.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.run.email}"
}

resource "google_project_iam_member" "sql" {
  count   = var.create_database ? 1 : 0
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.run.email}"
}

# ---------------------------------------------------------------- service

resource "google_cloud_run_v2_service" "app" {
  count               = local.deploy ? 1 : 0
  name                = var.name
  location            = var.region
  ingress             = "INGRESS_TRAFFIC_ALL"
  deletion_protection = false

  template {
    service_account                  = google_service_account.run.email
    max_instance_request_concurrency = 80
    timeout                          = "120s"
    scaling {
      min_instance_count = 0
      max_instance_count = var.max_instances
    }

    dynamic "volumes" {
      for_each = var.create_database ? [1] : []
      content {
        name = "cloudsql"
        cloud_sql_instance {
          instances = [google_sql_database_instance.db[0].connection_name]
        }
      }
    }

    containers {
      image = var.image
      ports {
        container_port = 8080
      }
      resources {
        limits            = { cpu = "1", memory = "512Mi" }
        cpu_idle          = true
        startup_cpu_boost = true
      }

      dynamic "env" {
        for_each = {
          SERVERLESS    = "true"
          BASE_URL      = local.base_url
          TRUST_PROXY   = "1"
          SIGNUP_MODE   = var.signup_mode
          BLOB_STORE    = "postgres"
          SMTP_HOST     = var.smtp.host
          SMTP_PORT     = tostring(var.smtp.port)
          SMTP_SECURITY = var.smtp.security
          SMTP_USERNAME = var.smtp.username
          SMTP_FROM     = var.smtp.from
          AI_PROVIDER   = var.ai.provider
          AI_MODEL      = var.ai.model
          AI_BASE_URL   = var.ai.base_url
        }
        content {
          name  = env.key
          value = env.value
        }
      }

      dynamic "env" {
        for_each = nonsensitive(toset([for k, v in local.secrets : k if v != ""]))
        content {
          name = env.value
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.app[env.value].secret_id
              version = "latest"
            }
          }
        }
      }

      dynamic "volume_mounts" {
        for_each = var.create_database ? [1] : []
        content {
          name       = "cloudsql"
          mount_path = "/cloudsql"
        }
      }

      startup_probe {
        http_get {
          path = "/health"
        }
        period_seconds    = 3
        failure_threshold = 20
      }
    }
  }

  depends_on = [google_secret_manager_secret_iam_member.run, google_secret_manager_secret_version.app]
}

# The application authenticates every request itself.
resource "google_cloud_run_v2_service_iam_member" "public" {
  count    = local.deploy ? 1 : 0
  name     = google_cloud_run_v2_service.app[0].name
  location = var.region
  role     = "roles/run.invoker"
  member   = "allUsers"
}

resource "google_cloud_scheduler_job" "jobs" {
  count    = local.deploy ? 1 : 0
  name     = "${var.name}-jobs"
  schedule = "*/30 * * * *"
  region   = var.region
  http_target {
    uri         = "${local.run_url}/internal/jobs"
    http_method = "POST"
    headers     = { Authorization = "Bearer ${random_password.jobs_secret.result}" }
  }
  depends_on = [google_project_service.apis]
}
