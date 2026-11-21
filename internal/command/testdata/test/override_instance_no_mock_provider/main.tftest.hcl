run "override_single_instance" {
  override_resource {
    target = terraform_data.x["one"]
    values = {
      output = "overridden"
    }
  }

  assert {
    condition     = terraform_data.x["one"].output == "overridden"
    error_message = "expected overridden instance output to be overridden"
  }

  assert {
    condition     = terraform_data.x["two"].output == "two"
    error_message = "expected non-overridden instance output to come from real provider"
  }
}
