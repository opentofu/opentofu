resource "test_resource" "foo" {
  value = "hello world"
}

variable "instances" {
  type = number
  default = 0
  validation {
    condition     = var.instances >= 0
    error_message = "The number of instances must be positive or zero"
  }
}

resource "test_resource" "bar" {
  value = "Hello!"
}
