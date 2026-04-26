# aws_instance is already defined inline in the test file; this should be skipped.
mock_resource "aws_instance" {
  defaults = {
    ami = "ami-from-file"
  }
}

# aws_vpc is not defined inline, so it should be loaded from this file.
mock_resource "aws_vpc" {
  defaults = {
    cidr_block = "10.0.0.0/16"
  }
}
