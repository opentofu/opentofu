variables {
  username = "test_user"
}

provider "test" {
  username = replace("${var.username}.${run.setup.domain}", ".", "@")
  password = replace(run.setup.password, "p", "P")
  data_prefix = "test"
  resource_prefix = "test"
}

run "setup" {
  module {
    source = "./first"
  }
}

run "validate" {
}
