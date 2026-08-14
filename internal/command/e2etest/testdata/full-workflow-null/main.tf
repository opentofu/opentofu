
variable "name" {
  default = "world"
}

# We're using cloudinit_config from the hashicorp/cloudinit provider here
# just because this test originally used template_file from hashicorp/template,
# but that provider is now deprecated and we want a replacement that's also
# purely local-only while still testing our ability to use a data resource
# type from an external provider plugin.
data "cloudinit_config" "test" {
  part {
    content  = "Hello, ${var.name}"
  }
}

resource "null_resource" "test" {
  triggers = {
    greeting = "${data.cloudinit_config.test.part[0].content}"
  }
}

output "greeting" {
  value = "${null_resource.test.triggers["greeting"]}"
}
