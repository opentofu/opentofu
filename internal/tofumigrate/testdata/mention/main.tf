terraform {
  required_providers {
    aws = {
      source  = "registry.terraform.io/hashicorp/aws"
    }
  }
}

provider "random" {}
provider "aws" {}

resource "random_id" "example" {
  byte_length = 8
}

resource "aws_instance" "example" {
  ami           = "abc"
  instance_type = "t2.micro"
}
