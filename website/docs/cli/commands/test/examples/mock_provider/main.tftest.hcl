// All resources and data sources provided by `aws.mock` provider
// will be mocked. Their values will be automatically generated.
mock_provider "aws" {
  alias = "mock"
}

// The same goes for `local` provider. Also, every `local_file`
// data source will have its `content` set to `test`.
mock_provider "local" {
  alias = "mock"
  mock_data "local_file" {
    defaults = {
      content = "test"
    }
  }
}

// Test if the bucket name is correctly passed to the aws_s3_bucket
// resource from the local file.
run "test" {
  providers = {
    // Use `aws.mock` and `local.mock` providers for this test run only.
    aws = aws.mock
    local = local.mock
  }

  assert {
    condition     = aws_s3_bucket.test.bucket == "test"
    error_message = "Incorrect bucket name: ${aws_s3_bucket.test.bucket}"
  }

  assert {
    condition     = output.example_output != ""
    error_message = "Empty output."
  }
}
