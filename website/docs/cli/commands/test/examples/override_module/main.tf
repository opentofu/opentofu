module "bucket_meta" {
  source = "./bucket_meta"
}

provider "aws" {
  region = "us-east-2"
}

resource "aws_s3_bucket" "test" {
  bucket = module.bucket_meta.name
  tags   = module.bucket_meta.tags
}
