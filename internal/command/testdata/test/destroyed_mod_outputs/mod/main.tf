variable "val" {
}

output "val" {
    value = test_resource.resource.id
}

resource "test_resource" "resource" {
  id    = "${var.val}_598318e0"
  value = var.val
}
