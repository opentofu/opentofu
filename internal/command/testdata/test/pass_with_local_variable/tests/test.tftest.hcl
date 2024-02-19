run "first" {
  command = apply
}

run "second" {
  command = plan

  module {
    source = "./tests/testmodule"
  }

  variables {
    foo = run.first.foo
  }
}