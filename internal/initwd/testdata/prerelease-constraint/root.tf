# We expect this test to download the version 0.0.2, the one before the
# specified version even with the equality because the specified version is a
# prerelease.
#
# Registry versions are:
# - 0.0.3-alpha.1
# - 0.0.2
# - 0.0.1

module "acctest" {
  source = "hashicorp/module-installer-acctest/aws"
  version = "<=0.0.3-alpha.1"
}
