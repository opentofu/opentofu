run "test" {
  module {
    source="../"
  }
  variables {
    name = "OpenTofu"
  }
  assert {
    condition = output.greeting == "Hello OpenTofu!"
    error_message = "Incorrect greeting: ${output.greeting}"
  }
}