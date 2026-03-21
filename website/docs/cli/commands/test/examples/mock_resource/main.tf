data "local_file" "bucket_name" {
  filename = "bucket_name.txt"
}

provider "aws" {
  region = "us-east-2"
}

data "aws_caller_identity" "current" {}

resource "aws_s3_bucket" "demo" {
  bucket = data.local_file.bucket_name.content
}
