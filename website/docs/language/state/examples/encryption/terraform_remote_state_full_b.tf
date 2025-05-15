terraform {
  encryption {
    # Note that the name of the key here is different:
    key_provider "pbkdf2" "my_key_renamed" {
      passphrase               = "OpenTofu has encryption"
      # Note the fixed encrypted_metadata_alias here:
      encrypted_metadata_alias = "certificates"
    }
    method "aes_gcm" "my_method_name" {
      keys = key_provider.pbkdf2.my_key_renamed
    }
    remote_state_data_sources {
      default {
        method = method.aes_gcm.my_method_name
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