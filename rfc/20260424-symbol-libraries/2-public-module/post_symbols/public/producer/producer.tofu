resource "tfcoremock_simple_resource" "entity" {
  string = "Hello World"
}

# Make sure any updates to the outputs of this module are added to producer.sym.hcl

output "id" {
  value = tfcoremock_simple_resource.entity.id
}

output "data" {
  value = {
    text = tfcoremock_simple_resource.entity.string
    other = tfcoremock_simple_resource.entity.integer
  }
}
