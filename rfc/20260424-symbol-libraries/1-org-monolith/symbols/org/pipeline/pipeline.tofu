symbols "common" {
  source = "../common"
}

variable "info" {
  type = symbols::common::info()
}

variable "input" {
  type = string
}
variable "output" {
  type = string
}

variable "transformer" {
  type = symbols::common::transformer()
}

# tfcoremock stands in for a cloud resource
resource "tfcoremock_simple_resource" "pipeline" {
  id = symbols::common::identity(var.info, "pipeline")
  string = "${var.input} -> ${var.transformer.id} -> ${var.output}"
}

module "policy" {
  source = "../policy"
  info = var.info

  target = tfcoremock_simple_resource.pipeline.id
}

output "id" {
  value = tfcoremock_simple_resource.pipeline.id
}
