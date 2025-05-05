variable "module_version" {
  type    = string
  default = null
}

module "foo" {
  source  = "./foo"
  version = var.module_version
} 