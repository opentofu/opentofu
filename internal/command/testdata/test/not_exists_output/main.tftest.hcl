run "not_exists" {
  assert {
    condition = output.something_that_does_not_exists == null
    error_message = "Should fail for Reference to undeclared output value"
  }
}
