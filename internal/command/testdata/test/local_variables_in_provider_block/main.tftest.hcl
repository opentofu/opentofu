variables {
  username = "u"
  password = "p"
}

provider "test" {
  username = var.username
  password = var.password
}

run "validate" {}


