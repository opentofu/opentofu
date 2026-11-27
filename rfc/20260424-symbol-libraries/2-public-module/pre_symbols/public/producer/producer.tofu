resource "tfcoremock_simple_resource" "entity" {
  string = "Hello World"
}

output "id" {
  value = tfcoremock_simple_resource.entity.id
}

output "data" {
  value = {
    text = tfcoremock_simple_resource.entity.string
    other = tfcoremock_simple_resource.entity.integer
  }
}
