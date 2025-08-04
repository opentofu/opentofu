variable "ephemeral_var" {
  type      = string
  ephemeral = true
}

variable "regular_var" {
  type      = string
}

# resource "test_instance" "foo" {
#   ami = "bar"
# }
