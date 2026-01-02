variables {
  foo = null
}

run "valid_when_null" {
  command = plan
}

run "valid_when_true" {
  command = plan
  variables {
    foo = {
      bar = true
    }
  }
}
