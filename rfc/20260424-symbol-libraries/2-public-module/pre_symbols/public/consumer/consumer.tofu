variable "produced" {
  type = object({
    id = string
    data = object({
      text = string
      other = number
    })
  })
}

resource "tfcoremock_simple_resource" "consumed" {
  string = jsonencode(var.produced)
}
