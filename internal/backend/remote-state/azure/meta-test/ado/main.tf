resource "time_static" "rg_timestamp" {}
data "azurerm_client_config" "current" {}

resource "random_string" "resource_suffix" {
  length  = 4
  special = false
  upper   = false
}

locals {
  storage_account_name = "acctestsa${random_string.resource_suffix.result}"
  resource_group_name  = "acctestRG-backend-${time_static.rg_timestamp.unix}-${random_string.resource_suffix.result}"
  container_name       = "acctestcont"
}

resource "azurerm_resource_group" "this" {
  name     = local.resource_group_name
  location = var.location
}


resource "azurerm_storage_account" "this" {
  name                     = local.storage_account_name
  resource_group_name      = azurerm_resource_group.this.name
  location                 = azurerm_resource_group.this.location
  account_tier             = "Standard"
  account_replication_type = "LRS"
}

resource "azurerm_storage_container" "this" {
  name                  = local.container_name
  storage_account_id    = azurerm_storage_account.this.id
  container_access_type = "private"
}

resource "azurerm_user_assigned_identity" "this" {
  location            = azurerm_resource_group.this.location
  name                = "open-tofu-test-identity"
  resource_group_name = azurerm_resource_group.this.name
}

resource "azurerm_role_assignment" "this" {
  scope                = azurerm_storage_account.this.id
  role_definition_name = "Storage Account Contributor"
  principal_id         = azurerm_user_assigned_identity.this.principal_id
}

resource "azurerm_role_assignment" "blob_contributor" {
  scope                = azurerm_storage_container.this.id
  role_definition_name = "Storage Blob Data Contributor"
  principal_id         = azurerm_user_assigned_identity.this.principal_id
}

resource "azuredevops_project" "this" {
  name            = "acctest-ado-project-${random_string.resource_suffix.result}"
  visibility      = "private"
  version_control = "Git"
  features = {
    repositories = "enabled"
    pipelines    = "enabled"
  }
}

resource "azuredevops_git_repository" "this" {
  project_id     = azuredevops_project.this.id
  name           = "acctest-repo-${random_string.resource_suffix.result}"
  default_branch = "refs/heads/main"
  initialization {
    init_type = "Clean"
  }
}

resource "azuredevops_git_repository_file" "this" {
  repository_id       = azuredevops_git_repository.this.id
  file                = "workflows/test.yml"
  content             = <<-EOT
trigger:
  branches:
    include:
      - main

pool:
  vmImage: ubuntu-latest

stages:
  - stage: AcceptanceTests
    jobs:
      - job: GoAcceptance
        steps:
          - task: AzureCLI@2
            displayName: 'Run Go acceptance tests'
            env:
              SYSTEM_ACCESSTOKEN: $(System.AccessToken)
            inputs:
              azureSubscription: azurerm-test
              scriptType: bash
              scriptLocation: inlineScript
              addSpnToEnvironment: true
              inlineScript: |
                set -euo pipefail
                SubId=$(az account show --query id --output tsv)

                export TF_AZURE_TEST=1
                export TF_ACC=1
                export ARM_CLIENT_ID=$servicePrincipalId
                export ARM_SUBSCRIPTION_ID=$SubId
                export ARM_TENANT_ID=$tenantId
                export ARM_USE_OIDC=true
                export TF_AZURE_TEST_STORAGE_ACCOUNT_NAME=${azurerm_storage_account.this.name}
                export TF_AZURE_TEST_RESOURCE_GROUP_NAME=${azurerm_resource_group.this.name}
                export TF_AZURE_TEST_CONTAINER_NAME=${azurerm_storage_container.this.name}
                chmod +x azure.test
                ./azure.test -test.v -test.run "TestAcc.*ADOWorkloadIdentity"
EOT
  branch              = "refs/heads/main"
  commit_message      = "Test pipeline file"
  overwrite_on_create = true
  author_name         = "acctest"
  author_email        = "acctest@opentofu.org"
}

resource "azuredevops_build_definition" "this" {
  project_id = azuredevops_project.this.id
  name       = "Test"
  path       = "\\"

  ci_trigger {
    use_yaml = true
  }

  repository {
    repo_type   = "TfsGit"
    repo_id     = azuredevops_git_repository.this.id
    branch_name = azuredevops_git_repository.this.default_branch
    yml_path    = "workflows/test.yml"
  }
}

resource "azuredevops_serviceendpoint_azurerm" "this" {
  project_id                             = azuredevops_project.this.id
  service_endpoint_name                  = "azurerm-test"
  service_endpoint_authentication_scheme = "WorkloadIdentityFederation"
  azurerm_spn_tenantid                   = data.azurerm_client_config.current.tenant_id
  azurerm_subscription_id                = data.azurerm_client_config.current.subscription_id
  azurerm_subscription_name              = "test"
  credentials {
    serviceprincipalid = azurerm_user_assigned_identity.this.client_id
  }
}

resource "azuredevops_pipeline_authorization" "this" {
  project_id  = azuredevops_project.this.id
  resource_id = azuredevops_serviceendpoint_azurerm.this.id
  type        = "endpoint"
  pipeline_id = azuredevops_build_definition.this.id
}

resource "azurerm_federated_identity_credential" "this" {
  parent_id = azurerm_user_assigned_identity.this.id
  name      = "acctest-ado-wif"
  audience = [
    "api://AzureADTokenExchange"
  ]
  issuer  = azuredevops_serviceendpoint_azurerm.this.workload_identity_federation_issuer
  subject = azuredevops_serviceendpoint_azurerm.this.workload_identity_federation_subject
}
