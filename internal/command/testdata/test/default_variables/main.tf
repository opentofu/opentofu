
variable "input" {
  type    = string
  default = "Hello, world!"
}

variable "another_input" {
  type = object({
    optional_string = optional(string, "type_default")
    optional_number = optional(number, 42)
  })
}
