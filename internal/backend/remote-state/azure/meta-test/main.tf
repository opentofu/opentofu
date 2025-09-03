data "azurerm_client_config" "current" {}

# Create an application and service account
resource "azuread_application" "tf_test_application" {
  display_name = "TF Test Application"
  owners       = [data.azurerm_client_config.current.object_id]
}

resource "azuread_service_principal" "tf_principal" {
  client_id                    = azuread_application.tf_test_application.client_id
  app_role_assignment_required = false
  owners                       = [data.azurerm_client_config.current.object_id]
}

# Role assignment, so that this can do anything in our tests.
resource "azurerm_role_assignment" "account_admin" {
  scope                = "/subscriptions/${data.azurerm_client_config.current.subscription_id}"
  role_definition_name = "Storage Account Contributor"
  principal_id         = azuread_service_principal.tf_principal.object_id
}

# Secret credentials
resource "azuread_application_password" "pw" {
  application_id = azuread_application.tf_test_application.id
}

resource "tls_private_key" "priv_key" {
  algorithm = "RSA"
  rsa_bits  = 4096
}

resource "tls_self_signed_cert" "cert" {
  private_key_pem = tls_private_key.priv_key.private_key_pem

  # Certificate expires after 2 weeks.
  validity_period_hours = 14 * 24

  # Generate a new certificate if Terraform is run within two
  # days of the certificate's expiration time.
  early_renewal_hours = 2 * 24

  # Reasonable set of uses for a server SSL certificate.
  allowed_uses = [
    "client_auth",
    "key_encipherment",
    "digital_signature",
    "server_auth",
  ]

  subject {
    common_name  = "myclientcertificate"
    organization = "LF Projects, LLC"
    province     = "CA"
    country      = "US"
  }
}
resource "azuread_application_certificate" "example" {
  application_id = azuread_application.tf_test_application.id
  type           = "AsymmetricX509Cert"
  value          = tls_self_signed_cert.cert.cert_pem
}


resource "random_string" "password" {
  length  = 24
  special = false
  upper   = true
}

resource "pkcs12_from_pem" "my_pkcs12" {
  password        = random_string.password.result
  cert_pem        = tls_self_signed_cert.cert.cert_pem
  private_key_pem = tls_private_key.priv_key.private_key_pem
  encoding        = "legacyRC2"
}

resource "local_file" "cert" {
  filename       = "certs.pfx"
  content_base64 = pkcs12_from_pem.my_pkcs12.result
}

module "msi" {
  source = "./msi"
  count  = var.use_msi ? 1 : 0

  location         = var.location
  ssh_pub_key_path = var.ssh_pub_key_path
}

locals {
  msi_extra_env_vars = !var.use_msi ? "" : <<EOT
export TF_AZURE_TEST_STORAGE_ACCOUNT_NAME=${module.msi[0].storage_account_name}
export TF_AZURE_TEST_RESOURCE_GROUP_NAME=${module.msi[0].resource_group_name}
export TF_AZURE_TEST_CONTAINER_NAME=${module.msi[0].container_name}
EOT
}
