# We expect this test to download the version 0.0.3-alpha.1
# the constraint matches the prerelease version available in
# the registry exactly.
#
# Registry versions are:
# - 0.0.3-alpha.1
# - 0.0.2
# - 0.0.1

module "acctest" {
  source = "hashicorp/module-installer-acctest/aws"
  version = "=0.0.3-alpha.1"
}
