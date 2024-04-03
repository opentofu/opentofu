terraform {
  encryption {
    key_provider "gcp_kms" "basic" {
      kms_encryption_key = "projects/local-vehicle-id/locations/global/keyRings/ringid/cryptoKeys/keyid"
      key_length = 32
    }
  }
}