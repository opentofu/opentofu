variable "policy_primary" {
  type = object({
    id = string
    owner = string
    actions = list(object({
      user = string
      action = string
    }))
  })
  validation {
    condition = var.policy_primary.id != ""
    error_message = "Document ID required"
  }
  validation {
    condition = length(var.policy_primary.actions) != 0
    error_message = "Document Actions required"
  }
}

variable "policy_secondary" {
  type = object({
    id = string
    owner = string
    actions = list(object({
      user = string
      action = string
    }))
  })
  validation {
    condition = var.policy_secondary.id != ""
    error_message = "Document ID required"
  }
  validation {
    condition = length(var.policy_secondary.actions) != 0
    error_message = "Document Actions required"
  }
}

# tfcoremock is standing in for a real cloud
resource "tfcoremock_simple_resource" "policy_primary" {
  string = jsonencode({for action in var.policy_primary.actions : "${var.policy_primary.id}-${action.user}" => action })
}

resource "tfcoremock_simple_resource" "policy_secondary" {
  string = jsonencode({for action in var.policy_secondary.actions : "${var.policy_secondary.id}-${action.user}" => action })
}
