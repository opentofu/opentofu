import {
  to = aws_instance.import
  id = 1
}

moved {
  from = aws_instance.moved_from
  to = aws_instance.moved_to
}

removed {
  from = aws_instance.removed
  lifecycle {
    destroy = false
  }
}