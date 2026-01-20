
terraform {
  required_providers {
    example = {
      source = "example/example"

      configuration_aliases = [
        example.a,
        example.b,
      ]
    }
  }
}
