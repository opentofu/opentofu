resource "aws_instance" "foo" {
  id = "baz"
  require_new = "new"

  lifecycle {
    destroy = false
    create_before_destroy = true
  }
}
