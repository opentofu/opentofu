// Configure the AWS provider to run fake credentials and without
// any validations. Not all providers support this, but when they
// do, you can run fully offline tests.
provider "aws" {
  access_key = "foo"
  secret_key = "bar"

  skip_credentials_validation = true
  skip_region_validation      = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true
}

run "test" {
  // Run in plan mode to skip applying:
  command = plan

  // Disable the refresh to prevent reaching out to the AWS API:
  plan_options {
    refresh = false
  }

  // Test if the bucket name is correctly passed to the aws_s3_bucket
  // resource:
  variables {
    bucket_name = "test"
  }
  assert {
    condition     = aws_s3_bucket.test.bucket == "test"
    error_message = "Incorrect bucket name: ${aws_s3_bucket.test.bucket}"
  }
}