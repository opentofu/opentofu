// when a resource is pointing to a provider that is missing required_providers definition, tofu does not show the warn
// about implicit reference of a provider
resource "aws_iam_role" "test" {
  assume_role_policy = "test"
  provider = asw.test
}