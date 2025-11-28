resource "aws_instance" "foo" {
  for_each = toset(["a", "b"])
  id = "baz"
  require_new = "new"

  lifecycle {
    destroy = false
  }
}
