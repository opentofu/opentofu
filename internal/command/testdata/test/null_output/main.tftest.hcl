run "null" {
  assert {
    condition = output.my_null_output == null
    error_message = "Should work"
  }
}