resource "local_file" "dont_create_me" {
    filename = "${path.module}/dont_create_me.txt"
    content = "101"
}
