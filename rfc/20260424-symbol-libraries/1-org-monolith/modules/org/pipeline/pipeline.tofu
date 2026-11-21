variable "info" {
  type = object({
    division   = string
    department = string
    team       = string
    project    = string
  })
  # validation rules
}

variable "input" {
  type = string
}
variable "output" {
  type = string
}

variable "transformer" {
  type = object({
    id = string
  })
}

# tfcoremock stands in for a cloud resource
resource "tfcoremock_simple_resource" "pipeline" {
  id = "${var.info.division}-${var.info.department}-${var.info.team}-${var.info.project}-pipeline"
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
