terraform {
    required_providers {
        test = {
            source = "registry.opentofu.org/hashicorp/test"
	    }
    }
}

provider "test" {
    alias = "test2" 
    test_string = "config"
    features {}
}

resource "test_object" "a" {
}