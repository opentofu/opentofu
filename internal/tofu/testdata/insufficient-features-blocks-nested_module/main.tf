terraform {
    required_providers {
        test = {
            source = "registry.opentofu.org/hashicorp/test"
	    }
    }
}

module "nested" {
  source = "./nested"
}
