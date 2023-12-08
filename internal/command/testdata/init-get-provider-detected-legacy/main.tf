// This should result in installing opentofu/foo
provider foo {}

// This will try to install opentofu/baz, fail, and then suggest
// terraform-providers/baz
provider baz {}

// This will try to install hashicrop/frob, fail, find no suggestions, and
// result in an error
provider frob {}

module "some-baz-stuff" {
  source = "./child"
}

module "dicerolls" {
  source = "acme/bar/random"
}
