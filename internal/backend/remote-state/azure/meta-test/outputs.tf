output "environment" {
  value     = <<-EOT
            export TF_AZURE_TEST_CLIENT_ID=${azuread_application.tf_test_application.client_id}
            export TF_AZURE_TEST_SECRET=${azuread_application_password.pw.value}
            export TF_AZURE_TEST_CERT_PATH=${local_file.cert.filename}
            export TF_AZURE_TEST_CERT_PASSWORD=${random_string.password.result}
            ${local.msi_extra_env_vars}
            EOT
  sensitive = true
}

output "ssh_instructions" {
  value = var.use_msi ? "ssh ${module.msi[0].ssh_username}@${module.msi[0].ssh_ip}" : "No VM, so no ssh into the VM. Set use_msi=true to get ssh info."
}

output "msi_env_vars" {
  value     = !var.use_msi ? "Set use_msi=true to get environment variable set" : <<-EOT
            Please set the following environment variables in your VM:
            export TF_AZURE_TEST=1
            export TF_ACC=1
            export ARM_LOCATION=centralus
            export ARM_SUBSCRIPTION_ID='${data.azurerm_client_config.current.subscription_id}'
            export ARM_TENANT_ID='${data.azurerm_client_config.current.tenant_id}'
            export TF_AZURE_TEST_STORAGE_ACCOUNT_NAME=${module.msi[0].storage_account_name}
            export TF_AZURE_TEST_RESOURCE_GROUP_NAME=${module.msi[0].resource_group_name}
            export TF_AZURE_TEST_CONTAINER_NAME=${module.msi[0].container_name}
  EOT
  sensitive = true
}
