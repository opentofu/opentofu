variable "used_mod_in" {
}
variable "not_used_in" {
  default = "default not_used_in"
}

output "mod_out" {
  value = var.used_mod_in
}