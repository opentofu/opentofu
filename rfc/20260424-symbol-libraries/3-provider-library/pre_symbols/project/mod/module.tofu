variable "entity" {
  type = object({
    id = string
  })
}

resource "tfcoremock_simple_resource" "bar" {
  string = var.entity.id
}
