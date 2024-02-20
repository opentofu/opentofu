variable "state_passphrase" {
	type = string
	sensitive = true
	description = "Passphrase used to decrypt older state files"
}


terraform {
	backend "local" {
		path = "foo.tfstate"
	}
	encryption {
		# Key Providers
		keys "awskms" "primary" {
			region = "us-east-1"
			key_id = "alias/tofu"
		}
		keys "passphrase" "legacy" {
			value = var.state_passphrase
		}

		# Encryption Methods
		method "aes256" "primary" {
			keys = awskms.primary
			flavor = "gcm"
		}
		method "aes256" "legacy" {
			keys = passphrase.legacy
			flavor = "foo"
		}

		state {
			method = aes256.primary
			fallback = aes256.legacy
		}
		plan {
			fallback = aes256.legacy
		}
	}
}
