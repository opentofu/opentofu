variable "info" {
  type = object({
    division   = string
    department = string
    team       = string
    project    = string
  })
  # validation rules
}

variable "program" {
  type = string
}

# tfcoremock stands in for a cloud resource
resource "tfcoremock_simple_resource" "execute" {
  id = "${var.info.division}-${var.info.department}-${var.info.team}-${var.info.project}-transformer-execute"
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
