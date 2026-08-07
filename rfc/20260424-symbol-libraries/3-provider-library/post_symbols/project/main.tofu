resource "tfcoremock_simple_resource" "entity" {}

module "mymod" {
  source = "./mod"
  entity = tfcoremock_simple_resource.entity
}
