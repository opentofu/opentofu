# XOR-based dual-custody key provider

This key provider combines two keys to create a dual-custody encryption key using XOR. This provider is meant for testing purposes only.

> [!WARNING]
> This file is not an end-user documentation, it is intended for developers. Please follow the user documentation on the OpenTofu website unless you want to work on the encryption code.

## Configuration

You can configure the key provider as follows. Note, the input keys must have the same length.

```hcl2
terraform {
    encryption {
        key_provider "pbkdf2" "a" {
            passphrase = "This is passphrase 1"
        }
        key_provider "pbkdf2" "b" {
            passphrase = "This is passphrase 2"
        }
        key_provider "xor" "myprovider" {
            a = key_provider.pbkdf2.a
            b = key_provider.pbkdf2.b
        }
    }
}
```