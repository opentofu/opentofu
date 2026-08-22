
symbols "lib" {
  source = "./local-lib"
}

variable "my_items" {
  type = symbols::lib::items()
  validation {
    condition = symbols::lib::non_empty(var.my_items)
    error_message = "One or more elements is empty"
  }
  // Not yet supported:
  // default = symbols.lib.default_items
}

variable "my_items_unchecked" {
  type = symbols::lib::items()
}

locals {
  # This will raise an error from within the function validation if var.my_items_unchecked contains empty elements
  my_items_checked = symbols::lib::assert_non_empty(var.my_items_unchecked)
}

output "checked" {
  value = [var.my_items, local.my_items_checked]
}

language {
  experiments = [symbol_libraries]
}
