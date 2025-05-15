removed {
	from = tfcoremock_simple_resource.example

	lifecycle {
		destroy = true
	}

	provisioner "local-exec" {
		when = destroy
		command = "echo 'execute against ${self.id}'"
	}
}