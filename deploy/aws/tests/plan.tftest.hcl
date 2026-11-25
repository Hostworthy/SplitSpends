# Plans the stack against mocked providers, so it needs no AWS account.

mock_provider "aws" {
  mock_data "aws_iam_policy_document" {
    defaults = { json = "{}" }
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
