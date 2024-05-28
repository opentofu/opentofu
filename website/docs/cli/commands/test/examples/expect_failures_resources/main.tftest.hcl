run "test-failure" {
  variables {
    # This healthcheck endpoint won't exist:
    health_endpoint = "/nonexistent"
  }

  expect_failures = [
    # We expect this to fail:
    check.health
  ]
}