resource "test_instance" "foo" {
  foo = "bar"
}

module "child" {
  source = "./child"
} 