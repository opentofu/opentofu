variables {
  username = "test_user"
}

provider "test" {
  username = "${var.username}@${run.setup.domain}"
  password = run.setup.password
}

run "setup" {
  module {
    source = "./first"
  }
}

run "assert_domain" {
  assert {
    condition = run.setup.domain == "d"
    error_message = "invalid value"
  }
}

run "assert_password" {
    assert {
        condition = run.setup.password == "p"
        error_message = "invalid value"
    }
}

run "assert_username" {
    assert {
        condition = var.username == "test_user"
        error_message = "invalid value"
    }
}
