mock_provider "test" {
  override_resource {
	target = test_resource.example
	values = {
	  computed_value = "overridden"
	}
  }
}

run "plan_with_import_block" {
  command = plan
  assert {
	condition = test_resource.example.computed_value == "overridden"
  }
}
