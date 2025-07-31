run "main" {
  command = plan

  variables {
    instances = -1
  }

  expect_failures = [
    var.instances,
  ]
}