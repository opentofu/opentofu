resource "aws_instance" "foo" {
  id = "baz"
  require_new = "new"

  lifecycle {
    destroy = true
  }
}
