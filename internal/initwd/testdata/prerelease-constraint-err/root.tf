# We expect this test to error out because there are no
# non-prerelease versions that match this constraint.
#
# Registry versions are:
# - 0.0.3-alpha.1
# - 0.0.2
# - 0.0.1

module "acctest" {
  source = "hashicorp/module-installer-acctest/aws"
  version = ">=0.0.3-alpha.1"
}
