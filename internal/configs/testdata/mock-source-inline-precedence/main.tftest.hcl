mock_provider "aws" {
  source = "./mocks"

  # This inline block should take precedence over the one in the mock file.
  mock_resource "aws_instance" {
    defaults = {
      ami = "ami-inline"
    }
  }
}

