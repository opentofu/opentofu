run "validate_test_resource" {
  assert {
    condition     = test_resource.testRes.value == "ValueFROMtests/tfvars"
    error_message = "invalid value"
  }
}
