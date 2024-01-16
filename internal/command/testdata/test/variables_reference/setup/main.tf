variable "content" {
  type = string
}
/*
resource "random_string" "file_name" {
  length           = 16
  special          = false
}

resource "local_file" "foo" {
  filename = random_string.file_name.result
  content  = var.content
}*/

output "file_name" {
  value = "output_value"
}
