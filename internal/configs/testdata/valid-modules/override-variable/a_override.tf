variable "fully_overridden" {
  default = "a_override"
  description = "a_override description"
  deprecated = "a_override deprecated"
  type = string
  ephemeral = true
}

variable "partially_overridden" {
  default = "a_override partial"
}
