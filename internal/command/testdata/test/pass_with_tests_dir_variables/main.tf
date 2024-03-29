variable "testVar" {
  type = string
}

resource "test_resource" "testRes" {
  value = var.testVar
}
