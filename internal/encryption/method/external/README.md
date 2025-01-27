# External encryption method


> [!WARNING]
> This file is not an end-user documentation, it is intended for developers. Please follow the user documentation on the OpenTofu website unless you want to work on the encryption code.

This directory contains the `external` encryption method. You can configure it like this:

```hcl
terraform {
  encryption {
    method "external" "foo" {
      keys = key_provider.some.provider
      encrypt_command = ["/path/to/binary", "arg1", "arg2"]
      decrypt_command = ["/path/to/binary", "arg1", "arg2"]
    }
  }
}
```

The external method must implement the following protocol:

1. On start, the method binary must emit the header line matching [the header schema](protocol/header.schema.json) on the standard output.
2. OpenTofu supplies the input metadata matching [the input schema](protocol/input.schema.json) on the standard input.
3. The method binary must emit the output matching [the output schema](protocol/output.schema.json) on the standard output.