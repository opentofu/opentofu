symbols "common" {
  source = "./"
}

variable "policy_primary" {
  type = symbols::common::policy_document()
}
variable "policy_secondary" {
  type = symbols::common::policy_document()
}

# tfcoremock is standing in for a real cloud
resource "tfcoremock_simple_resource" "policy_primary" {
  string = symbols::common::policy_document_json(var.policy_primary)
}

resource "tfcoremock_simple_resource" "policy_secondary" {
  string = symbols::common::policy_document_json(var.policy_secondary)
}
