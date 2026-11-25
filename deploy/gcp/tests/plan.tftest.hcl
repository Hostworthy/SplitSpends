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
