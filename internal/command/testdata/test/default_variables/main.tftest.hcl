
run "applies_defaults" {
  assert {
    condition     = var.input == "Hello, world!"
    error_message = "should have applied default value"
  }

  variables {
    another_input = {
      optional_string = "Hello, world!"
    }
  }

  assert {
    condition     = var.another_input.optional_string == "Hello, world!"
    error_message = "should have used custom value from test file"
  }

  assert {
    condition     = var.another_input.optional_number == 42
    error_message = "should have used default type value"
  }
}
