run "run" {
  command = apply

  assert {
    condition     = module.first.id != 0
    error_message = "Fail"
  }
}