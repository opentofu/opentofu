resource "local_file" "dont_create_me" {
    filename = "${path.module}/dont_create_me.txt"
    content = "101"
}

resource "local_file" "create_me" {
    filename = "${path.module}/create_me.txt"
    content = "101"
}

output "create_me_filename" {
    value = "main.tf"
}
