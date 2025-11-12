terraform {
  required_providers {
    local = {
      source  = "hashicorp/local"
      version = "2.5.3"
    }
  }
}

output "stuff" {
  value = provider::local::direxists("${path.module}/test-folder") ? "exists" : "does not exist"
}
