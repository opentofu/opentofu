module "mod" {
  source = "./mod"
}


resource "aws_instance" "c" {
  foo = "${module.mod.output}"
}
