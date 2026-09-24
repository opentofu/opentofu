mock_provider "test" {
  source="mocks"    
}

run "test" {
  assert {
    condition = test_resource.primary.computed_value == "bar"
    error_message = "Unexpected computed value"
  }
}
