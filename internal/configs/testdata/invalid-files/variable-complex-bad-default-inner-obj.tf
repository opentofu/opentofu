// https://github.com/opentofu/opentofu/issues/2394
// This validates the returned error message when the default value
// inner field type does not match the definition of the variable
variable "bad_type_for_inner_field" {
    type = map(object({
        field = bool
    }))

    default = {
        "mykey" = {
            field = "not a bool"
        }
    }
}
