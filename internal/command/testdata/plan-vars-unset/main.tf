variable "src" {
	type = string
}

module "mod" {
	source = "${var.src}"
}

resource "test_instance" "src" {
    value = "${var.src}"
}
