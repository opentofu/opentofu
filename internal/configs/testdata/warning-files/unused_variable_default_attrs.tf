variable "a" {
  type = object({
    a = string
    b = object({})
  })
  default = {
    a = "boop"
    b = {
      nope = "..." # WARNING: Object attribute is ignored
    }
    c = "hello" # WARNING: Object attribute is ignored
  }
}
