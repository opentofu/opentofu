resource "test_instance" "web" {
}

import {
  to = test_instance.web
  id = var.undefined
}
