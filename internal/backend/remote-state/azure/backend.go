// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package azure

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/backend/remote-state/azure/auth"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/legacy/helper/schema"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
)

const defaultTimeout = 300 // 5 minutes

// New creates a new backend for Azure remote state.
func New(enc encryption.StateEncryption) backend.Backend {
	s := &schema.Backend{
		Schema: map[string]*schema.Schema{
			"storage_account_name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "The name of the storage account.",
			},

			"container_name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "The container name.",
			},

			"key": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "The blob key.",
			},

			"metadata_host": {
				Type:          schema.TypeString,
				Optional:      true,
				Description:   "The Metadata URL which will be used to obtain the Cloud Environment.",
				DefaultFunc:   schema.EnvDefaultFunc("ARM_METADATA_HOST", nil),
				ConflictsWith: []string{"environment"},
			},

			"environment": {
				Type:          schema.TypeString,
				Optional:      true,
				Description:   "The Azure cloud environment.",
				DefaultFunc:   schema.EnvDefaultFunc("ARM_ENVIRONMENT", nil),
				ConflictsWith: []string{"metadata_host"},
			},

			"access_key": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The access key.",
				DefaultFunc: schema.EnvDefaultFunc("ARM_ACCESS_KEY", ""),
			},

			"sas_token": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "A SAS Token used to interact with the Blob Storage Account.",
				DefaultFunc: schema.EnvDefaultFunc("ARM_SAS_TOKEN", ""),
			},

			"snapshot": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Enable/Disable automatic blob snapshotting",
				DefaultFunc: schema.EnvDefaultFunc("ARM_SNAPSHOT", false),
			},

			"resource_group_name": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The resource group name.",
			},

			"client_id": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The Client ID.",
				DefaultFunc: schema.EnvDefaultFunc("ARM_CLIENT_ID", ""),
			},

			"endpoint": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "A custom Endpoint used to access the Azure Resource Manager API's.",
				Deprecated:  "This variable is unused and does not affect any execution. Please use environment or metadata host instead.",
			},

			"timeout_seconds": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "The timeout in seconds for initializing a client or retrieving a Blob or a Metadata from Azure.",
				DefaultFunc: schema.EnvDefaultFunc("ARM_TIMEOUT_SECONDS", defaultTimeout),
				ValidateFunc: func(v interface{}, _ string) ([]string, []error) {
					value, ok := v.(int)
					if !ok || value < 0 {
						return nil, []error{fmt.Errorf("timeout_seconds expected to be a non-negative integer")}
					}
					return nil, nil
				},
			},

			"subscription_id": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The Subscription ID.",
				DefaultFunc: schema.EnvDefaultFunc("ARM_SUBSCRIPTION_ID", ""),
			},

			"tenant_id": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The Tenant ID.",
				DefaultFunc: schema.EnvDefaultFunc("ARM_TENANT_ID", ""),
			},

			// Service Principal (Client Certificate) specific
			"client_certificate_password": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The password associated with the Client Certificate specified in `client_certificate_path`",
				DefaultFunc: schema.EnvDefaultFunc("ARM_CLIENT_CERTIFICATE_PASSWORD", ""),
			},
			"client_certificate_path": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The path to the PFX file used as the Client Certificate when authenticating as a Service Principal",
				DefaultFunc: schema.EnvDefaultFunc("ARM_CLIENT_CERTIFICATE_PATH", ""),
			},

			// Service Principal (Client Secret) specific
			"client_secret": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The Client Secret.",
				DefaultFunc: schema.EnvDefaultFunc("ARM_CLIENT_SECRET", ""),
			},

			// Managed Service Identity specific
			"use_msi": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Should Managed Service Identity be used?",
				DefaultFunc: schema.EnvDefaultFunc("ARM_USE_MSI", false),
			},
			"msi_endpoint": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The Managed Service Identity Endpoint.",
				DefaultFunc: schema.EnvDefaultFunc("ARM_MSI_ENDPOINT", nil),
				Deprecated:  "This configuration is now managed in a dependent library, not directly by OpenTofu. Please use the `MSI_ENDPOINT` environment variable to set the Managed Service Identity endpoint.",
			},

			// OIDC auth specific fields
			"use_oidc": {
				Type:        schema.TypeBool,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("ARM_USE_OIDC", false),
				Description: "Allow OIDC to be used for authentication",
			},
			"oidc_token": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("ARM_OIDC_TOKEN", ""),
				Description: "A generic JWT token that can be used for OIDC authentication. Should not be used in conjunction with `oidc_request_token`.",
			},
			"oidc_token_file_path": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("ARM_OIDC_TOKEN_FILE_PATH", ""),
				Description: "Path to file containing a generic JWT token that can be used for OIDC authentication. Should not be used in conjunction with `oidc_request_token`.",
			},
			"oidc_request_url": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{"ARM_OIDC_REQUEST_URL", "ACTIONS_ID_TOKEN_REQUEST_URL"}, ""),
				Description: "The URL of the OIDC provider from which to request an ID token. Needs to be used in conjunction with `oidc_request_token`. This is meant to be used for Github Actions.",
			},
			"oidc_request_token": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{"ARM_OIDC_REQUEST_TOKEN", "ACTIONS_ID_TOKEN_REQUEST_TOKEN"}, ""),
				Description: "The bearer token to use for the request to the OIDC providers `oidc_request_url` URL to fetch an ID token. Needs to be used in conjunction with `oidc_request_url`. This is meant to be used for Github Actions.",
			},

			// Feature Flags
			"use_azuread_auth": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Should OpenTofu use AzureAD Authentication to access the Blob?",
				DefaultFunc: schema.EnvDefaultFunc("ARM_USE_AZUREAD", false),
			},

			"use_cli": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Set to true if you want to use the Azure CLI to authenticate to Azure. Defaults to true.",
				DefaultFunc: schema.EnvDefaultFunc("ARM_USE_CLI", true),
			},
		},
	}

	result := &Backend{Backend: s, encryption: enc}
	result.Backend.ConfigureFunc = result.configure
	return result
}

type Backend struct {
	*schema.Backend
	encryption encryption.StateEncryption

	// The fields below are set from configure
	containerClient *container.Client
	// containerName is set here so that, in a unit test, we can
	// check that the container name is propagated correctly
	// from the configuration
	containerName string
	keyName       string
	snapshot      bool
	timeout       time.Duration
}

func (b *Backend) configure(ctx context.Context) error {
	if b.containerName != "" {
		return nil
	}

	// Grab the resource data
	data := schema.FromContextBackendConfig(ctx)
	b.containerName = data.Get("container_name").(string)
	b.keyName = data.Get("key").(string)
	b.snapshot = data.Get("snapshot").(bool)
	b.timeout = time.Duration(data.Get("timeout_seconds").(int)) * time.Second

	accessKey := data.Get("access_key").(string)
	sasToken := data.Get("sas_token").(string)
	useAzureADAuthentication := data.Get("use_azuread_auth").(bool)

	environment := data.Get("environment").(string)
	metadataHost := data.Get("metadata_host").(string)

	cloudConfig, storageSuffix, err := auth.CloudConfigFromAddresses(
		ctx,
		environment,
		metadataHost,
	)

	if err != nil {
		return err
	}

	config := &auth.Config{
		AzureCLIAuthConfig: auth.AzureCLIAuthConfig{
			CLIAuthEnabled: data.Get("use_cli").(bool),
		},
		ClientSecretCredentialAuthConfig: auth.ClientSecretCredentialAuthConfig{
			ClientID:     data.Get("client_id").(string),
			ClientSecret: data.Get("client_secret").(string),
		},
		ClientCertificateAuthConfig: auth.ClientCertificateAuthConfig{
			ClientCertificatePassword: data.Get("client_certificate_password").(string),
			ClientCertificatePath:     data.Get("client_certificate_path").(string),
		},
		OIDCAuthConfig: auth.OIDCAuthConfig{
			UseOIDC:           data.Get("use_oidc").(bool),
			OIDCToken:         data.Get("oidc_token").(string),
			OIDCTokenFilePath: data.Get("oidc_token_file_path").(string),
			OIDCRequestURL:    data.Get("oidc_request_url").(string),
			OIDCRequestToken:  data.Get("oidc_request_token").(string)},
		MSIAuthConfig: auth.MSIAuthConfig{
			UseMsi:   data.Get("use_msi").(bool),
			Endpoint: data.Get("msi_endpoint").(string),
		},
		StorageAddresses: auth.StorageAddresses{
			CloudConfig:      cloudConfig,
			ResourceGroup:    data.Get("resource_group_name").(string),
			StorageAccount:   data.Get("storage_account_name").(string),
			StorageContainer: b.containerName,
			StorageSuffix:    storageSuffix,
			SubscriptionID:   data.Get("subscription_id").(string),
			TenantID:         data.Get("tenant_id").(string),
		},
	}

	// MUST check storage account name and container name before trying to create a client.
	// We are going to be constructing URLs from these names, they should be restricted before we call those functions
	err = checkAccountAndContainerNames(config.StorageAccount, config.StorageContainer)
	if err != nil {
		return err
	}

	// Check for nonempty Storage Account Shared Access Key
	if accessKey != "" {
		containerClient, err := auth.NewContainerClientFromStorageAccessKey(ctx, config.StorageAddresses, accessKey)
		if err != nil {
			return err
		}

		b.containerClient = containerClient
		return nil
	}

	// Shared Access Key is now known to be empty

	// Check for nonempty SAS Token
	if sasToken != "" {
		containerClient, err := auth.NewContainerClientFromSAS(ctx, config.StorageAddresses, sasToken)
		if err != nil {
			return err
		}

		b.containerClient = containerClient
		return nil
	}

	// Shared Access Key and SAS Token are both empty

	// Get auth credentials
	authMethod, err := auth.GetAuthMethod(ctx, config)
	if err != nil {
		return err
	}

	// If we use Azure AD (Entra ID) Auth, we're done!
	// Just set up the client with these auth credentials
	if useAzureADAuthentication {
		authCred, err := authMethod.Construct(ctx, config)
		if err != nil {
			return err
		}
		bootstrapContainerClient, err := auth.NewContainerClient(ctx, config.StorageAddresses, authCred)
		if err != nil {
			return fmt.Errorf("error getting container client: %w", err)
		}
		b.containerClient = bootstrapContainerClient
		return nil
	}

	// We are not using Azure AD Auth
	// We're going to use these credentials to bootstrap obtaining the Shared Access Key credentials
	authCred, err := authMethod.Construct(ctx, config)
	if err != nil {
		return err
	}

	// We also call on the auth method to augment the configuration, to ensure resource group and subscription ID are present
	err = authMethod.AugmentConfig(ctx, config)
	if err != nil {
		return err
	}

	containerClient, err := auth.NewContainerClientWithSharedKeyCredential(ctx, config.StorageAddresses, authCred)
	if err != nil {
		return fmt.Errorf("error getting container client: %w", err)
	}

	b.containerClient = containerClient
	return nil
}

func checkAccountAndContainerNames(storageAccount, storageContainer string) error {
	accountPattern := regexp.MustCompile(`^[0-9a-z]{3,24}$`)
	containerPattern := regexp.MustCompile(`^[0-9a-z][0-9a-z\-]{1,61}[0-9a-z]$`)
	hyphenPattern := regexp.MustCompile(`\-\-`)
	if !accountPattern.Match([]byte(storageAccount)) {
		return errors.New("invalid storage account name: Azure requires a storage account name consists of 3-24 lowercase characters and numbers only. See documentation here: https://learn.microsoft.com/en-us/azure/azure-resource-manager/management/resource-name-rules#microsoftstorage")
	}
	if !containerPattern.Match([]byte(storageContainer)) {
		return errors.New("invalid storage container name: Azure requires a storage container name consists of 3-63 lowercase characters, numbers, and hyphens only. It cannot start or end with a hyphen. See documentation here: https://learn.microsoft.com/en-us/azure/azure-resource-manager/management/resource-name-rules#microsoftstorage")
	}
	if hyphenPattern.Match([]byte(storageContainer)) {
		return errors.New("invalid storage container name: Hyphens in a storage container name must be nonconsecutive. See documentation here: https://learn.microsoft.com/en-us/azure/azure-resource-manager/management/resource-name-rules#microsoftstorage")
	}
	return nil
}
