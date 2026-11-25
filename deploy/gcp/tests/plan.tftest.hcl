# Plans the stack against mocked providers, so it needs no Google Cloud project.

mock_provider "google" {}
mock_provider "random" {}

variables {
  project_id   = "my-project"
  database_url = "postgres://user:password@db.example.com/splitspends?sslmode=require"
  smtp         = { host = "smtp.example.com", from = "Split Spends <app@example.com>" }
  image        = "us-central1-docker.pkg.dev/my-project/splitspends/app:v1"
}

run "cloud_sql_scoped" {
  command = plan
  variables {
    create_database = true
    database_url    = ""
  }

  assert {
    condition     = google_project_iam_member.sql[0].condition[0].expression == "resource.name == 'projects/my-project/instances/splitspends-db' && resource.service == 'sqladmin.googleapis.com'"
    error_message = "The Cloud SQL client role must be limited to this stack's instance."
  }
}

# The secret variables depend on generated passwords, known only after apply.
run "settings" {
  command = apply
  variables {
    signup_allowed_domains = "example.com"
    jobs_schedule          = "0 * * * *"
  }

  assert {
    condition     = anytrue([for e in google_cloud_run_v2_service.app[0].template[0].containers[0].env : e.name == "SIGNUP_ALLOWED_DOMAINS" && e.value == "example.com"])
    error_message = "signup_allowed_domains must reach the service."
  }

  assert {
    condition     = google_cloud_scheduler_job.jobs[0].schedule == "0 * * * *"
    error_message = "jobs_schedule must set the scheduler."
  }

  assert {
    condition     = contains(keys(google_project_service.apis), "cloudbuild.googleapis.com")
    error_message = "The README builds with Cloud Build, so its API must be enabled."
  }
}

run "signup_mode_checked" {
  command = plan
  variables {
    signup_mode = "opne"
  }
  expect_failures = [var.signup_mode]
}
