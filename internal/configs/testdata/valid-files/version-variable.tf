variable "module_version" { default = "v1.0" }

module "foo" {
  source  = "mod/foo/foo"
  version = var.module_version
}
