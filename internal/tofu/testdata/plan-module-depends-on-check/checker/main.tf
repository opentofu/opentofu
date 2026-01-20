# A check block with a nested data source.
# This pattern caused a dependency cycle when used in modules
# that have depends_on relationships between them.
check "health" {
  data "test_data_source" "check" {
  }

  assert {
    condition     = data.test_data_source.check.id != ""
    error_message = "Check failed"
  }
}
