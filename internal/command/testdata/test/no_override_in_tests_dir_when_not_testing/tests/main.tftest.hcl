run "validate_test_resource" {
  command = plan
  assert {
    condition     = test_resource.testRes.value == "ValueFROMmain/tfvars"
    error_message = "invalid value"
  }
}
