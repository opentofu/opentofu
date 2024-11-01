# We expect this test to error out because multiple constraints are
# ANDed, and a prerelease constraint will not match non-prerelease
# versions.
#
# Registry versions are:
# - 0.0.3-alpha.1
# - 0.0.2
# - 0.0.1

module "acctest" {
  source = "hashicorp/module-installer-acctest/aws"
  version = "0.0.3-alpha.1, >=0.0.2"
}
