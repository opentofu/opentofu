# This is for testing that the implicitly defined providers cannot be fetched and the user is getting an info of the root cause
resource "nonexistingProv_res" "test1" {
}

module "testmod" {
  source = "./mod"
}
