function "leftpad" {
  parameter "str" { type = string }
  parameter "pad_width" { type = number }
  parameter "pad_char" {
    type = string
    variadic = true
    validation {
      condition = length(param.pad_char) <= 1
      error_message = "single pad char expected"
    }
    validation {
      condition = length(try(param.pad_char[0], " ")) != 1
      error_message = "expected single char"
    }
  }
  locals {
    pad_char = try(param.pad_char[0], " ")
    pad_size = max(0, param.pad_width - length(param.str))
    pad = join("", [for i in range(local.pad_size) : local.pad_char])
  }
  return = "${local.pad}${param.str}"
}
