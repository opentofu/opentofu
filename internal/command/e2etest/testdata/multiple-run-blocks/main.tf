resource "terraform_data" "provision" {
  connection {
    host = "localhost"
  }
  provisioner "remote-exec" {
    inline = ["echo test"]
  }
}
