// The calling test must set this variable for any command that
// will load this configuration.
variable "module_source" {
  type = string
}

module "test" {
  source = var.module_source
}
