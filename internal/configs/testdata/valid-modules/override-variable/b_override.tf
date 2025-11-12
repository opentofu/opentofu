variable "fully_overridden" {
  nullable = false
  default = "b_override"
  description = "b_override description"
  deprecated = "b_override deprecated"
  type = string
  ephemeral = false
}

variable "partially_overridden" {
  default = "b_override partial"
  deprecated = "b_override deprecated"
}
