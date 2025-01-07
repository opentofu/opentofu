run "first_apply" {
  variables {
    numbers = [ "a", "b" ]
  }

  assert {
    condition     = length(module.mod) == 2
    error_message = "Amount of module outputs is wrong"
  }
}

run "second_apply" {
  variables {
    numbers = [ "c", "d" ]
  }

  assert {
    condition     = length(module.mod) == 2
    error_message = "Amount of module outputs is wrong (persisted outputs?)"
  }
}
