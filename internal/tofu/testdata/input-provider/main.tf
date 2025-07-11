resource "aws_instance" "foo" {}
data "cloudflare_account" "bar" {}
ephemeral "azurerm_key_vault_secret" "baz" {}
