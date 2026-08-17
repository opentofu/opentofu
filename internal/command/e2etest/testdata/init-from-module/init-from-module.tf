terraform {
  required_providers {
    cloudinit = {
      source = "hashicorp/cloudinit"
    }
  }
}

data "cloudinit_config" "example" {
  part {
    content = "hello world"
  }
}
