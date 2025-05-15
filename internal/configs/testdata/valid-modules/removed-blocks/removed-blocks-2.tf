# One more removed block in a separate file just to make sure the
# appending of multiple files works properly.
removed {
  from = test.boop
  lifecycle {
    destroy = false
  }
}
