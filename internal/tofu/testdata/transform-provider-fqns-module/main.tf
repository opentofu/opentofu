terraform {
  required_providers {
    my-aws = {
      source = "opentofu/aws"
    }
  }
}

resource "aws_instance" "web" {
  provider = "my-aws"
}
