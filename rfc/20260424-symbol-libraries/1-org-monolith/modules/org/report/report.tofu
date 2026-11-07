variable "info" {
  type = object({
    division   = string
    department = string
    team       = string
    project    = string
  })
  # validation rules
}

variable "subject" {
  type = string
}

# tfcoremock stands in for a cloud resource
resource "tfcoremock_simple_resource" "report" {
  id = "${var.info.division}-${var.info.department}-${var.info.team}-${var.info.project}-report"
  string = var.subject
}

module "policy" {
  source = "../policy"
  info = var.info

  target = tfcoremock_simple_resource.report.id
}
