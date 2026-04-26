mock_resource "aws_instance" {
  defaults = {
    ami           = "ami-12345678"
    instance_type = "t2.micro"
  }
}

mock_data "aws_ami" {
  defaults = {
    id = "ami-12345678"
  }
}
