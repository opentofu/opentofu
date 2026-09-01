////// core:count-instead-enabled

provider "simple" {
  alias = "for_count_instead_enabled"
}

variable "sensitive_condition" {
  type = bool
  default = false
}

variable "condition" {
  type = bool
  default = true
}

// count expressions => reported
resource "simple_resource" "res1" {
  count = 0
}
data "simple_resource" "res2" {
  count = 1
}
ephemeral "simple_resource" "res3" {
  count = var.sensitive_condition ? 1 : 0
}
resource "simple_resource" "res4" {
  count = var.sensitive_condition ? 0 : 1
}
ephemeral "simple_resource" "res5" {
  count = var.sensitive_condition ? (var.condition ? 0 : 1) : 0
}
data "simple_resource" "res6" {
  count = var.sensitive_condition ? 0 : (var.condition ? 0 : 1)
}

// resource block count expressions => not reported
resource "simple_resource" "res7" {
  count = 2
}
resource "simple_resource" "res8" {
  count = var.sensitive_condition ? 1 : 2
}
resource "simple_resource" "res9" {
  count = var.sensitive_condition ? 0 : 2
}
