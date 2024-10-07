variable "src" {
	type = string
}

module "mod" {
	source = var.src
}

module "mod2" {
	source = var.src
}
