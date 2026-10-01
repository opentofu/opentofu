
module "child_a" {
  source  = "example.com/foo/bar_a/baz"
  version = ">= 1.0.0"
}

module "child_b" {
  source = "example.com/foo/bar_b/baz"
  version = ">= 1.0.0"
}

resource "test_resource" "res_test" {}
data "test_data" "data_test" {}
ephemeral "test_ephemeral" "ephemeral_test" {}