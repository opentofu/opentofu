terraform {
	required_providers {
		foo = {
			// This will conflict with the child modules hashicorp/foo
			source = "opentofu/foo"
		}
		dupe = {
			// This should not conflict with the child modules hashicorp/bar
			source = "bar"
		}
	}
}
module "some-baz-stuff" {
  source = "./child"
}
