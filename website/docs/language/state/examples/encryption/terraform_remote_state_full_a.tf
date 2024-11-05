terraform {
  encryption {
    key_provider "pbkdf2" "mykey" {
      passphrase               = "OpenTofu has encryption"
      # Note the fixed encrypted_metadata_alias here:
      encrypted_metadata_alias = "certificates"
    }
    method "aes_gcm" "mymethod" {
      keys = key_provider.pbkdf2.mykey
    }
    state {
      method = method.aes_gcm.mymethod
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