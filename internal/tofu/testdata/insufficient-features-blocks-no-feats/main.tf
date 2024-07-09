terraform {
    required_providers {
        test = {
            source = "registry.opentofu.org/hashicorp/test"
	    }
    }
}

provider "test" {
    test_string = "config"
}

resource "test_object" "a" {
}