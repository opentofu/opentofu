variable "input" {}

resource "test_resource" "foo" {
  value = var.input
}
