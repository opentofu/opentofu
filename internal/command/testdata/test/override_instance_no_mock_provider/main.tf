resource "terraform_data" "x" {
  for_each = toset(["one", "two"])
  input    = each.key
}
