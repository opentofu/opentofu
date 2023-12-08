terraform {
  required_providers {
    test = {
      source = "opentofu/test"
      configuration_aliases = [test.foo]
    }
  }
}

resource "test_resource" "foo" {
  provider = test.foo
}
