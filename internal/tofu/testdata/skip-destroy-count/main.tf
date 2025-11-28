resource "aws_instance" "foo" {
  count = 2
  id = "baz"
  require_new = "new"

  lifecycle {
    destroy = false
  }
}
