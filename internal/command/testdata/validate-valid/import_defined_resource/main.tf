resource "test_instance" "main" {
}

import {
  to = test_instance.main
  id = test_instance.reference.ami
}

resource "test_instance" "reference" {
}

