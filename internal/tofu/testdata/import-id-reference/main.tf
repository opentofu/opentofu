variable "the_id" {
  default = "123"
}

module "refmod" {
	source = "./mod"
}

import {
  to = module.refmod.aws_instance.foo
  id = var.the_id
}

