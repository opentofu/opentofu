symbols "lib" {
  source = "../producer"
}

variable "produced" {
  type = symbols::lib::producer()
}

resource "tfcoremock_simple_resource" "consumed" {
  string = jsonencode(var.produced)
}
