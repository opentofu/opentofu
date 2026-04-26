# This file should be ignored because a .tofumock.hcl alternative exists.
mock_resource "aws_instance" {
  defaults = {
    ami = "ami-from-tfmock"
  }
}
