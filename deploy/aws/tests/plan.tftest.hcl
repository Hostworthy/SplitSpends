# Plans the stack against mocked providers, so it needs no AWS account.

mock_provider "aws" {
  mock_data "aws_iam_policy_document" {
    defaults = { json = "{}" }
  }
  # The provider checks these ARNs when they are passed to other resources.
  mock_resource "aws_cloudfront_function" {
    defaults = { arn = "arn:aws:cloudfront::123456789012:function/splitspends-spa" }
  }
  mock_resource "aws_iam_role" {
    defaults = { arn = "arn:aws:iam::123456789012:role/splitspends" }
  }
  mock_resource "aws_cloudwatch_event_api_destination" {
    defaults = { arn = "arn:aws:events:us-east-1:123456789012:api-destination/splitspends-jobs/1" }
  }
  mock_resource "aws_apigatewayv2_api" {
    defaults = {
      api_endpoint  = "https://abc123.execute-api.us-east-1.amazonaws.com"
      execution_arn = "arn:aws:execute-api:us-east-1:123456789012:abc123"
    }
  }
  mock_resource "aws_cloudwatch_event_connection" {
    defaults = { arn = "arn:aws:events:us-east-1:123456789012:connection/splitspends-jobs/1" }
  }
}
mock_provider "archive" {}
mock_provider "random" {}

variables {
  database_url = "postgres://user:password@db.example.com/splitspends?sslmode=require"
  smtp         = { host = "smtp.example.com", from = "Split Spends <app@example.com>" }
  lambda_dir   = "tests"
  web_dir      = "tests"
}

run "default" {
  command = plan

  assert {
    condition     = aws_lambda_function.app.reserved_concurrent_executions == null
    error_message = "Without max_concurrency the function must not reserve concurrency."
  }
}

run "max_concurrency" {
  command = plan
  variables {
    max_concurrency = 10
  }

  assert {
    condition     = aws_lambda_function.app.reserved_concurrent_executions == 10
    error_message = "max_concurrency must reserve that many executions."
  }
}

run "max_concurrency_whole" {
  command = plan
  variables {
    max_concurrency = 0
  }
  expect_failures = [var.max_concurrency]
}

# Generated passwords are known only after apply, which the mocks simulate.
run "secrets" {
  command = apply

  assert {
    condition     = length(setintersection(keys(aws_lambda_function.app.environment[0].variables), ["DATABASE_URL", "SECRET_KEY", "JOBS_SECRET", "SMTP_PASSWORD", "AI_API_KEY"])) == 0
    error_message = "Secrets must not be in the function's environment."
  }

  assert {
    condition     = toset(keys(jsondecode(aws_secretsmanager_secret_version.app.secret_string))) == toset(["AI_API_KEY", "DATABASE_URL", "JOBS_SECRET", "SECRET_KEY", "SMTP_PASSWORD"])
    error_message = "The app secret must hold every server secret."
  }

  assert {
    condition     = jsondecode(aws_secretsmanager_secret_version.app.secret_string)["DATABASE_URL"] == var.database_url
    error_message = "The app secret must hold the database URL."
  }
}

run "signup_mode_checked" {
  command = plan
  variables {
    signup_mode = "opne"
  }
  expect_failures = [var.signup_mode]
}
