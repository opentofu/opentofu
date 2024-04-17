terraform {
	required_providers {
		example = {
			source = "opentofu/testfunctions"
			version = "1.0.0"
		}
	}
}

output "dummy" {
	value = provider::example::error()
}
