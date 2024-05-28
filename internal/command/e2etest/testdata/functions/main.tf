terraform {
	required_providers {
		example = {
			source = "opentofu/testfunctions"
			version = "1.0.0"
		}
	}
}

variable "number" {
	type  = number
	default = 1
	validation {
		condition = provider::example::echo(var.number) > 0
		error_message = "number must be > 0"
	}
}

output "dummy" {
	value = provider::example::echo("Hello Functions")
}
