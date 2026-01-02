variable "foo" {
  default = null
  type    = object({ bar = optional(bool, true) })
  validation {
    condition     = (var.foo == null || var.foo.bar)
    error_message = "Error! bar should be true!"
  }
}
