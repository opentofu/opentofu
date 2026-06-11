mock_provider "random" {}

run "happy_path" {
  command = plan
  assert {
    condition     = sleep_sleeper.test.string_wo == null
    error_message = "Incorrect content for sleep_sleeper.test.string_wo"

  }
}