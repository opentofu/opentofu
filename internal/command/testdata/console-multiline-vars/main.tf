variable "bar" {
    default = "baz"
}

variable "foo" {}

variable "counts" {
  type = map(any)
  default = {
    "lalala" = 1,
    "lololo" = 2,
  }
}
