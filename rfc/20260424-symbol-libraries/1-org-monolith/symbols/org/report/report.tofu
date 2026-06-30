symbols "common" {
  source = "../common"
}

variable "info" {
  type = symbols::common::types(info)
}

variable "subject" {
  type = string
}

# tfcoremock stands in for a cloud resource
resource "tfcoremock_simple_resource" "report" {
  id = symbols::common::identity(var.info, "report")
  string = var.subject
}

module "policy" {
  source = "../policy"
  info = var.info

  target = tfcoremock_simple_resource.report.id
}
