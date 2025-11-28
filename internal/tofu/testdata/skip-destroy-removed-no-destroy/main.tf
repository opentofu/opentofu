removed {
  from = aws_instance.foo

  lifecycle {
    destroy = false
  }
}

