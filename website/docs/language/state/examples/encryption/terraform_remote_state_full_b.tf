terraform {
  encryption {
    # Note that the name of the key here is different:
    key_provider "pbkdf2" "mykeyrenamed" {
      passphrase               = "OpenTofu has encryption"
      # Note the fixed encrypted_metadata_alias here:
      encrypted_metadata_alias = "certificates"
    }
    method "aes_gcm" "mymethod" {
      keys = key_provider.pbkdf2.mykeyrenamed
    }
    remote_state_data_sources {
      default {
        method = method.aes_gcm.mymethod
      }
    }
  }
}


data "terraform_remote_state" "cert" {
  backend = "local"
  config = {
    # Refer to the other project here:
    path = "../a/terraform.tfstate"
  }
}

output "cert" {
  # Use data from the other project by referencing it as follows:
  value = data.terraform_remote_state.cert.outputs.cert_pem
}