variable "with_empty_deprecated" {
  type = string
  description = "variable that is having the deprecated field specified but is having no value"
  deprecated = ""
}

variable "with_deprecated_only_spaces" {
  type = string
  description = "variable that is having the deprecated field specified only with blank characters"
  deprecated = "   "
}

variable "without_deprecated" {
  type = string
  description = "no deprecated field defined so it should work fine"
}