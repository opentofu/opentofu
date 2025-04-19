// When calling a module and providing a value for a deprecated variable, OpenTofu should warn about that
module "foo-call" {
  source = "./modfoo"
  foo    = "foo given value"
}

// When calling a module and NOT providing a value for a deprecated variable, OpenTofu should skip warning about it
module "foo-call-no-var" {
  source = "./modfoo"
}

// When calling a module and providing a NULL value for a deprecated variable, OpenTofu should skip warning about it
// This is due to the way OpenTofu handles null values.
// For more information, check the considerations about the `null` value from the docs: https://opentofu.org/docs/language/expressions/types/#types
module "foo-call-null" {
  source = "./modfoo"
  bar    = null
}
