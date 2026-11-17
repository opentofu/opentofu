resource "aws_instance" "foo" {
  id = "bar"
}

module "nested" {
  source = "./submodule"
}
