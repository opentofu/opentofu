mock_provider "test" {
  override_resource {
	target = test_resource.example
	values = {
	  computed_value = "provider-overridde"
	}
  }
}


run "run_override" {
  override_resource {
	target = test_resource.example
	values = {
	  computed_value = "run-override"
	}
  }

  command = plan
  assert {
	condition = test_resource.example.computed_value == "run-override"
  }
}
