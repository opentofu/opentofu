terraform {
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "4.62.1"
    }
    azuread = {
      source  = "hashicorp/azuread"
      version = "3.4.0"
    }
    tls = {
      source  = "hashicorp/tls"
      version = "4.1.0"
    }
    pkcs12 = {
      source  = "chilicat/pkcs12"
      version = "0.2.5"
    }
    azuredevops = {
      source  = "microsoft/azuredevops"
      version = "1.13.0"
    }
  }
}

provider "azurerm" {
  resource_provider_registrations = "none"
  features {}
}

provider "azuread" {
}

provider "azuredevops" {
}
