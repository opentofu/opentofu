resource "local_file" "test" {
  filename = "${path.module}/test.txt"
  content  = "Hello world!"
}