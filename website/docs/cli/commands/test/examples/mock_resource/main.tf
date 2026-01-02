data "local_file" "bucket_name" {
  filename = "bucket_name.txt"
}

provider "aws" {
  region = "us-east-2"
}

resource "aws_s3_bucket" "test" {
  bucket = data.local_file.bucket_name.content
}
