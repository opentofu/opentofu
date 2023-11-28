variable "bucket_name" {}

provider "aws" {
  region = "us-east-2"
}

resource "aws_s3_bucket" "test" {
  bucket = var.bucket_name
}