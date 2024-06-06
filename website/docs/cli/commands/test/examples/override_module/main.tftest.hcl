// All the module configuration will be ignored for this
// module call. Instead, the `outputs` object will be used
// to populate module outputs.
override_module {
  target = module.bucket_meta
  outputs = {
    name = "test"
    tags = {
      Environment = "Test Env"
    }
  }
}

// Test if the bucket name is correctly passed to the aws_s3_bucket
// resource from the module call.
run "test" {
  // S3 bucket will not be created in AWS for this run,
  // but it's available to use in both tests and configuration.
  override_resource {
    target = aws_s3_bucket.test
  }

  assert {
    condition     = aws_s3_bucket.test.bucket == "test"
    error_message = "Incorrect bucket name: ${aws_s3_bucket.test.bucket}"
  }

  assert {
    condition     = aws_s3_bucket.test.tags["Environment"] == "Test Env"
    error_message = "Incorrect `Environment` tag: ${aws_s3_bucket.test.tags["Environment"]}"
  }
}
