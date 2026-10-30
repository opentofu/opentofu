variable "policy" {
  type = object({
    id = string
    owner = string
    actions = list(object({
      user = string
      action = string
    }))
  })
  validation {
    condition = var.policy.id != ""
    error_message = "Document ID required"
  }
  validation {
    condition = length(var.policy.actions) != 0
    error_message = "Document Actions required"
  }
}

# tfcoremock is standing in for a real cloud
resource "tfcoremock_simple_resource" "policy" {
  string = jsonencode({for action in var.policy.actions : "${var.policy.id}-${action.user}" => action })
}

output "policy" {
  value = tfcoremock_simple_resource.policy
}
