variable "policy_primary" {
  type = object({
    id = string
    owner = string
    actions = list(object({
      user = string
      action = string
    }))
  })
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
}

module "primary" {
  source = "./common"
  policy = var.policy_primary
}
module "secondary" {
  source = "./common"
  policy = var.policy_secondary
}
