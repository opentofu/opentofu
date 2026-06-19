terraform {
  encryption {
    key_provider "aws_kms" "basic" {
      kms_key_id = "a4f791e1-0d46-4c8e-b489-917e0bec05ef"
      region     = "us-east-1"
      key_spec   = "AES_256"

      # Optional: key-value pairs sent to KMS.
      # Useful for key policy conditions and CloudTrail audit logging.
      encryption_context = {
        Environment = "production"
        Application = "myapp"
      }
    }
  }
}
