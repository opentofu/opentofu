variable "id" {
  type = string
}

locals {
  items = toset(["a", "b"])
}

resource "test_instance" "this" {
  for_each = local.items
}
