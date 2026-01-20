output "environment" {
  value     = <<-EOT
            export TF_AZURE_TEST_CLIENT_ID=${azuread_application.tf_test_application.client_id}
            export TF_AZURE_TEST_CLIENT_SECRET=${azuread_application_password.pw.value}
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

output "aks_kubectl_instructions" {
  value = !var.use_aks_workload_identity ? "" : <<-EOT
  Run the following on your machine to finish kubernetes setup
  az aks get-credentials --name "${module.aks[0].cluster_name}" --resource-group "${module.aks[0].resource_group_name}"
  cat <<EOF | kubectl apply -f -
  apiVersion: v1
  kind: ServiceAccount
  metadata:
    annotations:
      azure.workload.identity/client-id: "${module.aks[0].az_client_id}"
    name: "${module.aks[0].ksa_name}"
    namespace: "default"
  ---
  apiVersion: v1
  kind: Pod
  metadata:
    name: shell-demo
    namespace: default
    labels:
      azure.workload.identity/use: "true"
  spec:
    serviceAccountName: "${module.aks[0].ksa_name}"
    containers:
    - name: alpine
      image: alpine
      command: [ "/bin/sh", "-c", "--" ]
      args: [ "while true; do sleep 30; done;" ]
    hostNetwork: true
    dnsPolicy: Default
  EOF
EOT
}

output "aks_env_vars" {
  value     = !var.use_aks_workload_identity ? "Set use_aks_workload_identity=true to get environment variable set" : <<-EOT
            Please set the following environment variables in your pod:
            export TF_AZURE_TEST=1
            export TF_ACC=1
            export ARM_LOCATION=centralus
            export ARM_SUBSCRIPTION_ID='${data.azurerm_client_config.current.subscription_id}'
            export ARM_TENANT_ID='${data.azurerm_client_config.current.tenant_id}'
            export TF_AZURE_TEST_STORAGE_ACCOUNT_NAME=${module.aks[0].storage_account_name}
            export TF_AZURE_TEST_RESOURCE_GROUP_NAME=${module.aks[0].resource_group_name}
            export TF_AZURE_TEST_CONTAINER_NAME=${module.aks[0].container_name}
  EOT
  sensitive = true
}
