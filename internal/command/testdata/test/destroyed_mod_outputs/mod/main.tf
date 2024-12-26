variable "val" {
}

output "val" {
    value = "${var.val}_${test_resource.resource.id}"
}

resource "test_resource" "resource" {
  id = "id-${var.val}"
  value = var.val
}
