// Use a mocked AWS provider so no real AWS API calls are made.
mock_provider "aws" {
  // Provide stable defaults for computed values on a RESOURCE type.
  mock_resource "aws_s3_bucket" {
    defaults = {
      // These are computed attributes for aws_s3_bucket.
      id  = "demo-bucket-id"
      arn = "arn:aws:s3:::demo-bucket-id"
    }
  }

  // Provide stable defaults for computed values on a data source type.
  mock_data "aws_caller_identity" {
    defaults = {
      account_id = "123456789012"
      arn        = "arn:aws:iam::123456789012:root"
      user_id    = "AIDEXAMPLEUSERID"
    }
  }
}

run "defaults_from_mock_resource_and_mock_data" {
  command = plan

  // Assert resource computed attributes come from mock_resource.defaults
  assert {
    condition     = aws_s3_bucket.demo.id == "demo-bucket-id"
    error_message = "Expected aws_s3_bucket.demo.id to come from mock_resource defaults."
  }

  assert {
    condition     = aws_s3_bucket.demo.arn == "arn:aws:s3:::demo-bucket-id"
    error_message = "Expected aws_s3_bucket.demo.arn to come from mock_resource defaults."
  }

  // Assert data source computed attributes come from mock_data.defaults
  assert {
    condition     = data.aws_caller_identity.current.account_id == "123456789012"
    error_message = "Expected account_id to come from mock_data defaults."
  }

  assert {
    condition     = data.aws_caller_identity.current.arn == "arn:aws:iam::123456789012:root"
    error_message = "Expected arn to come from mock_data defaults."
  }
}
