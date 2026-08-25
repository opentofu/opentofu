# STACKIT KMS Key Provider

> [!WARNING]
> This file is not an end-user documentation, it is intended for developers. Please follow the user documentation on the OpenTofu website unless you want to work on the encryption code.

This folder contains the STACKIT KMS key provider. Users configure a STACKIT KMS key here to encrypt and decrypt state and plan data.

## Configuration

You can configure this key provider by specifying the following options:

```hcl2
terraform {
    encryption {
        key_provider "stackit_kms" "myprovider" {
            project_id  = "00000000-0000-0000-0000-000000000000"
            region      = "eu01"
            key_ring_id = "11111111-1111-1111-1111-111111111111"
            key_id      = "22222222-2222-2222-2222-222222222222"
            key_length  = 32
        }
    }
}
```

## Key Provider Options

- `project_id`, `region`, `key_ring_id`, `key_id`: identify the STACKIT KMS
  key. All required. `region` is passed as a call parameter on every
  request, not as client-level configuration: KMS is a global-endpoint API,
  and the SDK rejects a client-level region for it.
- `key_version`: optional. Pins the key version that wraps the data
  encryption key. If unset, the provider resolves the highest-numbered
  active, non-disabled version once at configuration time and uses it for
  the lifetime of that key provider instance. The resolved version is
  always stored alongside the ciphertext in the state's encryption
  metadata, so decrypting older state keeps working after key rotation.
- `key_length`: required. Size, in bytes, of the generated data encryption
  key.
- `service_account_key`, `service_account_key_path`, `service_account_token`,
  `private_key`, `private_key_path`, `endpoint`: optional authentication
  and endpoint overrides. When unset, the STACKIT SDK's credential chain
  resolves `STACKIT_*` environment variables and the credentials file
  (`~/.stackit/credentials.json`) automatically.

## STACKIT SDK Notes

This package depends on
`github.com/stackitcloud/stackit-sdk-go/services/kms/v1api`, not the root
`github.com/stackitcloud/stackit-sdk-go/services/kms` package, which is
deprecated. In `v1api`, the encrypt/decrypt request and response `Data`
fields are base64-encoded strings, not `[]byte`, so this package encodes
and decodes them manually (see `provider.go`).

`v1api`'s generated request-builder types (`ApiEncryptRequest` and similar)
carry unexported fields with no getters, so mocks outside the `v1api`
package cannot inspect them. `keyManagementClient` (`provider.go`) keeps
those SDK-shaped types out of the mockable surface: `apiClient` adapts
`v1api.DefaultAPI` to it, and tests (`mock_test.go`) implement
`keyManagementClient` directly with a hand-written mock.

## Testing Against a Real Key

`go test` runs entirely against a mock by default. `TestKeyProviderLive`
runs a real encrypt/decrypt round trip instead, when `TF_ACC=1` or
`TF_KMS_TEST=1` is set, together with `TF_STACKIT_KMS_PROJECT_ID`,
`TF_STACKIT_KMS_REGION`, `TF_STACKIT_KMS_KEY_RING_ID`, and
`TF_STACKIT_KMS_KEY_ID` identifying an existing
`symmetric_encrypt_decrypt`/`aes_256_gcm` key with at least one active
version. STACKIT credentials come from the usual `STACKIT_*` environment
variables.

## State Snapshotting and Key Usage

OpenTofu generates a new encryption key each time it stores encrypted
data, rather than reusing one key across snapshots. This keeps every
state snapshot cryptographically independent, at the cost of creating
more keys in STACKIT KMS than a per-project or per-environment key
scheme would.
