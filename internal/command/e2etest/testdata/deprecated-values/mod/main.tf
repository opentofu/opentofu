variable "input" {
  default = "world"
  deprecated = "This var is deprecated"
}

output "output" {
  value = "${var.input}"
  deprecated = "this output is deprecated"
}
