terraform {
  encryption {
    key_provider "external" "ovhcloud" {
      command = [
        "opentofu-kms-ovhcloud",
        # Required. UUID of the service key used to wrap/unwrap the data key.
        # Falls back to the KMS_KEY_ID environment variable.
        "--key-id", "00000000-0000-0000-0000-000000000000",
        # Optional. Name attached to the generated data key (KMS_KEY_NAME).
        # "--key-name", "my-tofu-data-key",
        # Optional. Data key size in bits: 128, 192 or 256 (KMS_KEY_BITS). Default: 256.
        # "--key-bits", "256",
      ]
    }

    method "aes_gcm" "ovhcloud" {
      keys = key_provider.external.ovhcloud
    }

    state {
      method = method.aes_gcm.ovhcloud
    }
    plan {
      method = method.aes_gcm.ovhcloud
    }
  }
}
