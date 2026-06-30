symbols "common" {
  source = "../common"
}

variable "info" {
  type = symbols::common::types(info)
}

variable "target" {
  type = string
}

# tfcoremock stands in for a cloud resource
resource "tfcoremock_simple_resource" "document" {
  id = "${var.target}-policy"
  string = jsonencode({
    org = symbols::common::org(var.info)
    owner = symbols::common::owner(var.info) 
    target = var.target
  })
}

output "id" {
  value = tfcoremock_simple_resource.document.id
}
