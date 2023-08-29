run "test" {
  command         = plan
  expect_failures = [
    check.check
  ]
}
