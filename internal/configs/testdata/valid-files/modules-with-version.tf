locals {
  module_version = null
}
module "foo" {
  source  = "./foo"
  version = local.module_version
}

locals {
  module_version_set = "1.0.0"
}
module "foo_remote" {
  source  = "hashicorp/foo/bar"
  version = local.module_version_set
}