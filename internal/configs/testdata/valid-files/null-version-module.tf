locals {
  module_version = null
}
module "foo" {
  source  = "./foo"
  version = local.module_version
} 