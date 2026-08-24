typedef "items" {
    type = list(string)
}

function "non_empty" {
  parameter "in" {
    type = symbols::items() 
  }
  return = alltrue([for x in param.in: length(x) != 0])
}

function "assert_non_empty" {
  parameter "in" {
    type = symbols::items()
    validation {
      condition = symbols::non_empty(param.in)
      error_message = "One or more of the elements in ${jsonencode(param.in)} is empty"
    }
  }
  return = param.in
}

values {
    default_items = ["foo", "bar", "baz"]
}
