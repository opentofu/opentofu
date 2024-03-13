variable "iter" {
  type = string
}

resource "tfcoremock_simple_resource" "simple" {
  string = "helloworld ${var.iter}"
}
