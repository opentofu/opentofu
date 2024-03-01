# AES-GCM encryption method

This folder contains the state encryption implementation of the AES-GCM encryption method.

This is implemented following the guidance of the following document: ([NIST SP 800-38D](https://csrc.nist.gov/pubs/sp/800/38/d/final)).


## Configuration

You can configure the encryption by specifying the following method block:

```hcl2
terraform {
  encryption {
    method "aes_gcm" "mymethod" {
      # Pass the key provider with a 16, 24, or 32 byte encryption key here:
      keys = key_provider.someprovider.somename
      
      # Leave the AAD empty unless needed. Pass as a list of bytes if needed:  
      aad  = [1,2,3,4,...]
    }
  }
}
```

| Field               | Description                                                                                                                                                                                      |
|---------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `keys` (*required*) | Encryption and decryption key, as well as key provider metadata.                                                                                                                                 |
| `aad`               | Additional Authenticated Data. This data is stored along the encrypted form and authenticated. The AAD value of the encrypted form must match the configuration, otherwise the decryption fails. |

## Key rotation

AES-GCM keys have a limited lifetime of `2^32` blocks, equaling roughly 64 GB of data that can be encrypted before the keys should be considered compromised. Users should not use AES-GCM without a regular key rotation mechanism or a key derivation function such as PBKDF2 as a key provider.

## Encryption vs. Authentication

The AES-GCM implementation protects data at rest from being accessed. It does not, however, protect against malicious actors reusing old data (replay attacks) to compromise the integrity of the system. Users with the need for payload authentication should rotate their key and/or AAD frequently to ensure that old data cannot be used in this manner.

## Implementation notes

### Additional Authenticated Data (AAD)

The AAD in AES-GCM is a general-purpose authenticated, but not encrypted field in the encrypted payload. The Go implementation only supports using this field as a canary value, rejecting decryption if the value mismatches. AES-GCM would support using this field as a means to store data. Since Go does not support it, neither do we.

### Future compatibility

The current nonce and tag size recommendations may change in the future. The configuration fields here are allowed to permit users to to decrypt their old state files.

### Panics

The current Go implementation of AES-GCM uses `panic()` to handle some input errors. To work around that, the `errorhandling.Safe` function is used to capture the panic and turn it into an error.