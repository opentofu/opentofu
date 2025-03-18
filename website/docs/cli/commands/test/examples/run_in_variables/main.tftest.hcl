run "setup" {
  module {
    source = "./setup"
  }
}

run "test" {
  variables {
    file_name_from_setup = run.setup.file_name
  }
}
