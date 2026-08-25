terraform {
  encryption {
    key_provider "stackit_kms" "basic" {
      project_id  = "00000000-0000-0000-0000-000000000000"
      region      = "eu01"
      key_ring_id = "11111111-1111-1111-1111-111111111111"
      key_id      = "22222222-2222-2222-2222-222222222222"
      key_length  = 32

      # Optional: pin a specific key version. If omitted, the highest-numbered
      # active version is resolved automatically and stored alongside the
      # ciphertext, so decrypting older state keeps working after rotation.
      # key_version = 1
    }
  }
}
