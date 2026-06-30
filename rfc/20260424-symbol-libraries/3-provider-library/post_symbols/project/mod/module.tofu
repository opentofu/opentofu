symbols "tfcoremock"{
  source = "../../tfcoremock" # github.com/user/terraform-provider-tfcoremock//types
}

variable "entity" {
  type = symbols::tfcoremock::types(simple_resource)
}

resource "tfcoremock_simple_resource" "bar" {
  string = var.entity.id
}
