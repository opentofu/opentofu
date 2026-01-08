# Minimal reproducer for https://github.com/opentofu/opentofu/issues/3644
# Bug requires ignore_changes on a block (not a simple attribute), here I use network_interface

terraform {
  required_providers {
    vsphere = {
      source  = "vmware/vsphere"
      version = "~> 2.14"
    }
  }
}

resource "vsphere_virtual_machine" "vm" {
  name             = "x"
  resource_pool_id = "x"
  network_interface { network_id = "x" }

  lifecycle {
    ignore_changes = [network_interface]
  }
}
