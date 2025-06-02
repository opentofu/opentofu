# Test configuration with artificial delays to see the TUI

resource "null_resource" "slow1" {
  provisioner "local-exec" {
    command = "sleep 2"
  }
}

resource "null_resource" "slow2" {
  provisioner "local-exec" {
    command = "sleep 2"
  }
}

resource "null_resource" "slow3" {
  depends_on = [null_resource.slow1]
  provisioner "local-exec" {
    command = "sleep 2"
  }
}

resource "null_resource" "slow4" {
  depends_on = [null_resource.slow2]
  provisioner "local-exec" {
    command = "sleep 2"
  }
}

output "completion_message" {
  value = "All slow resources created!"
}