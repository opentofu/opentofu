terraform {
    required_providers {
        foo = {
            source = "opentofu/bar"
            configuration_aliases = [ foo.bar ]
        }
        bar = {
            source = "opentofu/foo"
        }
    }
}

resource "foo_resource" "resource" {}

resource "bar_resource" "resource" {}
