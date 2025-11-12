mock_provider "test" {
  alias = "by_region"
  for_each = toset(var.regions)
  mock_resource "test_resource" {
    defaults = {
      computed_value = "bar"      
      object_attr = {
        string_attr = "bar"
      }
    }
  }
}

variables {
  regions = ["us-east-1", "us-west-2"]
}

run "test" {
  assert {
    condition = test_resource.primary["us-east-1"].computed_value == "bar"
    error_message = "Unexpected computed value"
  }
}

run "test2" {
  assert {
    condition = test_resource.primary["us-west-2"].computed_value == "bar"
    error_message = "Unexpected computed value"
  }
}
