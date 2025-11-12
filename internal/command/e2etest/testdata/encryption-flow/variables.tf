// When the migration to encrypted plan and state is wanted,
// in case the passphrase for the encryption is given via
// -var/-var-file, it is recommended to add the variable
// handling that to the configuration before adding the
// encryption configuration. Apply this, and only after start
// with the encryption configuration.
variable "passphrase" {
  type      = string
  sensitive = true
}

locals {
  key_length = sensitive(32)
}