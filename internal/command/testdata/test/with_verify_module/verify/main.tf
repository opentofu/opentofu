variable "id" {
  type = string
}

data "test_data_source" "resource_data" {
  id = var.id
}

resource "test_resource" "another_resource" {
  id = "hi"
}