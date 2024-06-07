// This data source will not be called for any run
// in this `.tftest.hcl` file. Instead, `values` object
// will be used to populate `content` attribute. Other
// attributes and blocks will be automatically generated.
override_data {
  target = data.local_file.bucket_name
  values = {
    content = "test"
  }
}

// Test if the bucket name is correctly passed to the aws_s3_bucket
// resource from the local file.
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
}
