variables {
  username = "test_user"
}

provider "test" {
  username = var.username
  password = run.setup.password
}

run "setup" {
  module {
    source = "./first"
  }
}

run "validate" {}
