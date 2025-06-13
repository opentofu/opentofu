variable "use_msi" {
  default     = false
  type        = bool
  description = "Set this to generate the VM infrastructure and managed service identity authorizations required to run the MSI tests."
}

variable "location" {
  default     = "centralus"
  type        = string
  description = "The location for the VM used for MSI testing. Only relevant if use_msi is set to true."
}

variable "ssh_pub_key_path" {
  default     = "~/.ssh/id_rsa.pub"
  type        = string
  description = "The file path on the local file system where this user's public SSH key is located. This is for ssh-ing into the VM used for MSI testing, and so it is only relevant if use_msi is set to true."
}
