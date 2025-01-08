# PBKDF passphrase key provider

> [!WARNING]
> This file is not an end-user documentation, it is intended for developers. Please follow the user documentation on the OpenTofu website unless you want to work on the encryption code.

This folder contains the code for the PBKDF2 passphrase key provider. The user can enter a passphrase and the key provider will generate `[]byte` keys of a given length and will record the salt in the encryption metadata.

## Configuration

You can configure this key provider by specifying the following options:

```hcl2
terraform {
    encryption {
        key_provider "pbkdf2" "myprovider" {
            passphrase = "enter a long and complex passphrase here"
            
            # Alternatively, chain the passphrase from an upstream key provider:
            chain = key_provider.other.provider
            
            # Adapt the key length to your encryption method needs,
            # check the method documentation for the right key length
            key_length = 32
            
            # Provide the number of iterations that should be performed.
            # See https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html#pbkdf2
            # for recommendations
            iterations = 600000 
	
            # Pick the hashing function. Can be sha256 or sha512.
            hash_function = "sha512"
	        
            # Pick the salt length in bytes.
            salt_length = 32
        }
    }
}
```