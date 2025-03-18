run "setup" {
  module {
    source = "./setup"
  }
}

run "test" {
  variables {
    filename_from_setup = run.setup.filename
  }

  # more assertions to run
}
