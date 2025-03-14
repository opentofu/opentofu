variables {
  username = "test_user"
}

provider "test" {
  # Invalid ref to run.setup.username
  username = run.setup.username
  password = run.setup.password

}

run "setup" {
  module {
    source = "./first"
  }
}

run "validate" {}
