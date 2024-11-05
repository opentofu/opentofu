variable "testVar" {
  type = string
}

resource "test_instance" "testRes" {
  ami = var.testVar
}
