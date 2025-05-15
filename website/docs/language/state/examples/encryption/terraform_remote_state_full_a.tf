terraform {
  encryption {
    key_provider "pbkdf2" "my_key_provider_name" {
      passphrase               = "OpenTofu has encryption"
      # Note the fixed encrypted_metadata_alias here:
      encrypted_metadata_alias = "certificates"
    }
    method "aes_gcm" "my_method_name" {
      keys = key_provider.pbkdf2.my_key_provider_name
    }
    state {
      method = method.aes_gcm.my_method_name
    }
  }
}

resource "tls_private_key" "webserver" {
  algorithm   = "ED25519"
}

resource "tls_self_signed_cert" "webserver" {
  private_key_pem = tls_private_key.webserver.private_key_pem

  subject {
    common_name  = "someserver.opentofu.org"
    organization = "OpenTofu"
  }

  validity_period_hours = 24*365*10

  allowed_uses = [
    "key_encipherment",
    "digital_signature",
    "server_auth",
  ]
}

output "cert_pem" {
  value = tls_self_signed_cert.webserver.cert_pem
}