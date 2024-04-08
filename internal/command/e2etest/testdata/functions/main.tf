terraform {
	required_providers {
		example = {
			source = "yantrio/helpers"
			version = "1.0.0"
		}
	}
}

output "dummy" {
	value = provider::example::echo("Hello Functions")
}
