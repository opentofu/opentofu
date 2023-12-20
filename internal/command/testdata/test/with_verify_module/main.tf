variable "id" {
  type = string
}

variable "value" {
  type = string
}

resource "test_resource" "resource" {
  id = var.id
  value = var.value
}