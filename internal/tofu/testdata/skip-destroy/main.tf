resource "aws_instance" "foo" {
  lifecycle {
    destroy = false
  }
}
