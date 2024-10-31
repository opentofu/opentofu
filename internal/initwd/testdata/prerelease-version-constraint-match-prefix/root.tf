# We expect this test to download the requested version because it is an exact
# match for a prerelease version.

module "acctest" {
  source = "hashicorp/module-installer-acctest/aws"
  version = "v0.0.3-alpha.1"
}
