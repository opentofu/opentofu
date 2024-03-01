resource "null_resource" "a" {
  provisioner "local-exec" {
    when = invalid # ERROR: Invalid "when" keyword
    on_failure = invalid # ERROR: Invalid "on_failure" keyword
    lifecycle {} # ERROR: Reserved block type name in provisioner block
  }
}