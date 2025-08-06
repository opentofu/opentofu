module "test" {
  source = "./module"
}

# This creates a SINGLE value with MULTIPLE deprecated marks at different paths
# Each field gets its own PathValueMark with ONLY a deprecation mark
# This pattern triggers the slice modification bug in unmarkDeepWithPathsDeprecated
# https://github.com/opentofu/opentofu/issues/3104
locals {
  all_deprecated = {
    a = module.test.out1
    b = module.test.out2
    c = module.test.out3
  }
}

# Force evaluation by using in an output
output "trigger" {
  value = local.all_deprecated
}