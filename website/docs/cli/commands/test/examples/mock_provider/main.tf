data "local_file" "bucket_name" {
  filename = "bucket_name.txt"
}

provider "aws" {
  region = "us-east-2"
}

resource "aws_s3_bucket" "test" {
  bucket = data.local_file.bucket_name.content
}

output "example_output" {
  value = provider::local::direxists("${path.module}/dir")
}

// `required_providers` block is necessary for calling
// provider-defined functions.
terraform {
  required_providers {
    local = {
      source  = "hashicorp/local"
      version = ">= 2.0"
    }
  }
}
