terraform {
  required_providers {
    test = {
      source = "hashicorp/test"
    }
  }
}

import {
  to = test_resource.example
  id = "existing-id"
}

resource "test_resource" "example" {
  value = "from-config"
}
