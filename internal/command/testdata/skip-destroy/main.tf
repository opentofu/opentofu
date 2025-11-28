resource "test_instance" "foo" {
  id = "baz"

  lifecycle {
    destroy = false
  }
}
