run "override_nonexistent_instance" {
  override_resource {
    target = terraform_data.x["zzz"]
    values = {
      output = "overridden"
    }
  }

  assert {
    condition     = terraform_data.x["one"].output == "one" && terraform_data.x["two"].output == "two"
    error_message = "expected all instances to use real provider when override target does not match"
  }
}
