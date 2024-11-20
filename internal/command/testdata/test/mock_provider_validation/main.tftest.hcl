mock_provider "test" {
  mock_resource "test_resource" {
    defaults = {
      computed_value = "bar"      
    }
  }
}

run "test" {
  assert {
    condition = test_resource.primary.computed_value == "bar"
    error_message = "Unexpected computed value"
  }
}
