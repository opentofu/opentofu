run "not_exists" {
  assert {
    condition = output.something_that_does_not_exist == null
    error_message = "Should fail for Reference to undeclared output value"
  }
}
