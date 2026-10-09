
# This could be automatically generated from `tofu providers schema -json`
typedef "simple_resource" {
  # TODO description = "A simple resource that holds optional attributes for the five basic types: `bool`, `number`, `string`, `float`, and `integer`."
  type = object({
    bool = optional(bool)       # An optional boolean attribute, can be true or false.
    float = optional(number)    # An optional float attribute.
    id = string
    integer = optional(number)  # An optional integer attribute.
    number = optional(number)   # An optional number attribute, can be an integer or a float.
    string = string             # An optional string attribute.
  })
}

