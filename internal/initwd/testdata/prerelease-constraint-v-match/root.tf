# We expect this test to download the requested version because it is an exact
# match for a prerelease version. The "v" prefix should be tolerated.
#
# Registry versions are:
# - 0.0.3-alpha.1
# - 0.0.2
# - 0.0.1

module "acctest" {
  source = "hashicorp/module-installer-acctest/aws"
  version = "v0.0.3-alpha.1"
}
