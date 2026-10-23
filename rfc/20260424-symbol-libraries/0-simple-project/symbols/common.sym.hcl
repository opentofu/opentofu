typedef "policy_document" {
  type = object({
    id = string
    owner = string
    actions = list(object({
      user = string
      action = string
    }))
  })
}

function "validate_policy_document" {
  parameter "document" {
    type = symbols::types(policy_document)
    validation {
      condition = param.document.id != ""
      error_message = "Document ID required"
    }
    validation {
      condition = length(param.document.actions) != 0
      error_message = "Document Actions required"
    }
  }
  return = param.document
}

function "policy_document_json" {
  parameter "document" {
    type = symbols::types(policy_document)
  }
  locals {
    validated = symbols::validate_policy_document(param.document)
    formatted = {for action in local.validated.actions : "${local.validated.id}-${action.user}" => action }
  }
  return = jsonencode(local.formatted)
}
