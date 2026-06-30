symbols "common" {
  source = "../common"
}

variable "info" {
  type = symbols::common::types(info)
}

variable "program" {
  type = string
}

# tfcoremock stands in for a cloud resource
resource "tfcoremock_simple_resource" "execute" {
  id = symbols::common::identity(var.info, "transformer-execute")
  string = var.program
}

module "policy" {
  source = "../policy"
  info = var.info

  target = tfcoremock_simple_resource.execute.id
}

output "id" {
  value = tfcoremock_simple_resource.execute.id
}
