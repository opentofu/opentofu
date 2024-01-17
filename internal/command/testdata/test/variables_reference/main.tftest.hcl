variables {
  content = "some value"
}

run "setup" {
  module {
    source = "./setup"
  }
}

run "test" {
  variables {
    file_name = run.setup.file_name
  }
}
