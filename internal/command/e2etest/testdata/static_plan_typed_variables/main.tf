variable "state_path" {}

variable "src" {
	type = string
}

terraform {
	backend "local" {
		path = var.state_path
	}
}

module "mod" {
	source = var.src
}

output "out" {
	value = module.mod.out
}
