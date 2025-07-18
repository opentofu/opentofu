## Proposal 

Create a new field for direct conditional enabling on resources:

```
resource "aws_instance" "example" {
  # ...

  lifecycle {
    enabled = var.enable_server
  }
}
```

## Implementation 

1. Add support on `validate`
2. Add support on `plan` 
3. Add support on `apply`

## Errors when resource is not enabled 


## Solutions to refer to it 

## Open questions

1. Can we use unknown values as expressions on the `enabled`?
1. How to migrate from existing count managed resources to use `enabled`?
