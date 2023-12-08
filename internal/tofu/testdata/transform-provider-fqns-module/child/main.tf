terraform {
  required_providers {
    your-aws = {
      source = "opentofu/aws"
    }
  }
}

resource "aws_instance" "web" {
  provider = "your-aws"
}
