variable "val" {
}

output "val" {
    value = "${var.val}_${test_resource.resource.id}"
}

resource "test_resource" "resource" {
  id = "598318e0"
  value = var.val
}
