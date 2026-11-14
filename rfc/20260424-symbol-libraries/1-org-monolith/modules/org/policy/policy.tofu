variable "info" {
  type = object({
    division   = string
    department = string
    team       = string
    project    = string
  })
  # validation rules
}

variable "target" {
  type = string
}

# tfcoremock stands in for a cloud resource
resource "tfcoremock_simple_resource" "document" {
  id = "${var.target}-policy"
  string = jsonencode({
    org = "${var.info.division}_${var.info.department}"
    owner = "${var.info.team}_${var.info.project}"
    target = var.target
  })
}

output "id" {
  value = tfcoremock_simple_resource.document.id
}
