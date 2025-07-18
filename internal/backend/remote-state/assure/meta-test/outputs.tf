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
