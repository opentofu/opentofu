variable "resource_name" {
    type = string
    description = "a variable"
}

provider "local" {
    alias = "${var.resource_name}"
}

module "bar" {
    source = "./${var.resource_name}"
}

resource "local_file" "test" {
  content  = "lala"
  filename = "./${var.resource_name}/test"
}
