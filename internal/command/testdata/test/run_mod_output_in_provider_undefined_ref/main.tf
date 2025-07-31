variable "username" {
    type    = string
    default = "user"
}

resource "test_resource" "foo" {
  value = var.username
}
