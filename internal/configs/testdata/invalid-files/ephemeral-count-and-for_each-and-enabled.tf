ephemeral "test" "foo" {
  lifecycle {
    enabled = true
  }
  count    = 2
  for_each = ["a"]
}
