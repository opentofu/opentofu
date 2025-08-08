terraform {
  required_providers {
    aws = {}
  }
}

variable "bucket_name" {
  type    = string
  default = "bucket-prod"
}

output "stuff" {
  value = provider::aws::arn_build("aws", "s3", "", "", var.bucket_name)
}
