# Test fixture for non-existent submodule issue
# This references the test module with a non-existent submodule path

module "non_existent_submodule" {
  source = "registry.opentofu.org/hashicorp/module-installer-acctest/aws//modules/non-existent"
  version = "0.0.1"
}