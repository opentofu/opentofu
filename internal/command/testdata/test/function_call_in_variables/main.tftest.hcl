variables {
  input = upper("test_value")
  joined = join("-", ["test", "values"])
}

run "validate_function_calls" {
  assert {
    condition = var.input == "TEST_VALUE"
    error_message = "upper() function did not work in variables"
  }
  
  assert {
    condition = var.joined == "test-values"
    error_message = "join() function did not work in variables"
  }
}