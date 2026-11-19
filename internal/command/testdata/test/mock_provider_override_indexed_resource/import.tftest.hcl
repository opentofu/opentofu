mock_provider "test" {
  override_resource {
    target = test_resource.example
    values = {
      computed_value = "provider overridden"
    }
  }
}

run "provider_overriden" {
  command = plan
  assert {
    condition     = output.computed_values == ["provider overridden", "provider overridden"]
    error_message = "invalid value for output"
  }
}

run "override_indexed_resource" {
  override_resource {
    target = test_resource.example[0]
    values = {
      computed_value = "run overridden 1"
    }
  }
  override_resource {
    target = test_resource.example[1]
    values = {
      computed_value = "run overridden 2"
    }
  }
  command = plan
  assert {
    condition     = output.computed_values == ["run overridden 1", "run overridden 2"]
    error_message = "invalid value for output"
  }
}
