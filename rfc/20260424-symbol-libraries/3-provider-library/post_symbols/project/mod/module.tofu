symbols "tfcoremock"{
  source = "../../tfcoremock" # github.com/user/terraform-provider-tfcoremock//types
}

variable "entity" {
  type = symbols::tfcoremock::simple_resource()
}

resource "tfcoremock_simple_resource" "bar" {
  string = var.entity.id
}
