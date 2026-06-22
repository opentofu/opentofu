terraform {
  encryption {
    key_provider "gcp_kms" "basic" {
      kms_encryption_key = "projects/local-vehicle-id/locations/global/keyRings/ringid/cryptoKeys/keyid"
      key_length         = 32

      # Optional: base64-encoded additional authenticated data sent to GCP KMS.
      additional_authenticated_data = base64encode("my-aad-value")
    }
  }
}
