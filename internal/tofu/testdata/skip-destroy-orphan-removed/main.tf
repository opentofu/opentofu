removed {
  from = aws_instance.skip_destroy_not_set

  lifecycle {
    destroy = true
  }
}

removed {
  from = aws_instance.skip_destroy_set

  lifecycle {
    destroy = false
  }
}
