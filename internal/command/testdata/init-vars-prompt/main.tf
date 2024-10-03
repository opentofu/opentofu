variable "resource_name" {
    type = string
    description = "a variable"
}

module "bar" {
    source = "./${var.resource_name}"
}
