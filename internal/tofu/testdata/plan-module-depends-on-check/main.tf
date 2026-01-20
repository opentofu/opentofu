# Regression test for https://github.com/opentofu/opentofu/issues/3060
# "Depending on a module with a check in it causes a dependency cycle"
#
# When module.dependent has depends_on = [module.base], and both modules
# use a shared submodule containing a check block with a nested data source,
# this should not cause a dependency cycle.

module "base" {
  source = "./base"
}

module "dependent" {
  source     = "./dependent"
  depends_on = [module.base]
}
