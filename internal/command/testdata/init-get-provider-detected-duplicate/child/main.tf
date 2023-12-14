terraform {
	required_providers {
		dupechild = {
			source = "hashicorp/bar"
		}
	}
}

// This will try to install hashicorp/foo
provider foo {}
