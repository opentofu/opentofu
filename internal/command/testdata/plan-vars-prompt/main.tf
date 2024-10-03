variable "resource_name" {
    type = string
    description = "a variable"
}

resource "test_instance" "foo" {
  ami = "bar"

  # This is here because at some point it caused a test failure
  network_interface {
    device_index = 0
    description  = "${var.resource_name}"
  }
}