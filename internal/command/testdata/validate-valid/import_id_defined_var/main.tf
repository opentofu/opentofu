variable "defined" {
  type = string
  default = "example_id"
}

import {
  to = test_instance.web
  id = var.defined
}
