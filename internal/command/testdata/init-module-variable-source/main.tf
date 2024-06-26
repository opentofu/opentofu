variable "src" {
	type = string
}

module "mod" {
	source = var.src
}
