variable "input" {
  type       = string
  default    = "val"
  deprecated = "this is local deprecated"
}
variable "input2" {
  type       = string
  default    = "val2"
  deprecated = "this is local deprecated2"
}

output "modout1" {
  value      = var.input
  deprecated = "output deprecated"
}
output "modout2" {
  value      = var.input2
  deprecated = "output deprecated"
}
