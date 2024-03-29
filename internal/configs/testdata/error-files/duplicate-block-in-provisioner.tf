resource "null_resource" "a" {
  provisioner "local-exec" {
    connection {}
    connection {} # ERROR: Duplicate connection block

    _ {}
    _ {} # ERROR: Duplicate escaping block
  }
}