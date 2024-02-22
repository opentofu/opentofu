# AES-GCM encryption method

This folder contains the state encryption implementation of the AES-GCM encryption method ([NIST SP 800-38D](https://csrc.nist.gov/pubs/sp/800/38/d/final)).

## Configuration

You can configure the encryption by specifying the following method block:

```hcl2
terraform {
  encryption {
    method "aes_gcm" "mymethod" {
      key        = "key here" # Pass your 16, 24, or 32 byte encryption key here.
      aad        = ""         # Leave empty unless needed
      nonce_size = 12         # Minimum: 1, do not change unless needed
      tag_size   = 16         # Valid values: 12-16, do not change unless needed
    }
  }
}
```

| Field              | Default | Description                                                                                                                                                                                      |
|--------------------|---------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `key` (*required*) |         | Your encryption key in binary format, likely referencing a key provider. Must be 16, 24, or 32 bytes long.                                                                                       |
| `aad`              |         | Additional Authenticated Data. This data is stored along the encrypted form and authenticated. The AAD value of the encrypted form must match the configuration, otherwise the decryption fails. |
| `nonce_size`       | 12      | Size of the Initialization Vector for AES-GCM. This field should not be changed and is provided for future compatibility.                                                                        |
| `tag_size`         | 16      | Size of the authentication tag (12-16 bytes). This field should not be changed and is provided for future compatibility.                                                                         |

## Key rotation

AES-GCM keys have a limited lifetime of `2^32` blocks, equaling roughly 64 GB of data that can be encrypted before the keys should be considered compromised. Users should rotate keys well before this limit is reached.

## Encryption vs. Authentication

The current AES-GCM implementation protects data at rest from being accessed. It does not, however, protect against malicious actors reusing old data (replay attacks) to compromise the integrity of the system. Users with the need for payload authentication should rotate their key and/or AAD frequently to ensure that old data cannot be used in this manner.

## Implementation notes

### AAD

The AAD in AES-GCM is a general-purpose authenticated, but not encrypted field in the encrypted payload. The Go implementation only supports using this field as a canary value, rejecting decryption if the value mismatches. AES-GCM would support using this field as a means to store data. Since Go does not support it, neither do we.

### Future compatibility

The current nonce and tag size recommendations may change in the future, but users should still be able to decrypt their old state files. This is why these fields exist.

### Panics

The current Go implementation of AES-GCM uses `panic()` to handle some input errors. To work around that, the `errorhandling.Safe` function is used to capture the panic and turn it into an error.