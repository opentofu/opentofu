// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package azure

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/2017-03-09/resources/mgmt/resources"
	armStorage "github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2021-01-01/storage"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/hashicorp/go-azure-helpers/authentication"
	"github.com/hashicorp/go-azure-sdk/sdk/auth"
	authWrapper "github.com/hashicorp/go-azure-sdk/sdk/auth/autorest"
	"github.com/hashicorp/go-azure-sdk/sdk/environments"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/version"
	"github.com/tombuildsstuff/giovanni/storage/2018-11-09/blob/blobs"
	"github.com/tombuildsstuff/giovanni/storage/2018-11-09/blob/containers"
)

type ArmClient struct {
	// These Clients are only initialized if an Access Key isn't provided
	groupsClient          *resources.GroupsClient
	storageAccountsClient *armStorage.AccountsClient
	containersClient      *containers.Client
	blobsClient           *blobs.Client

	// azureAdStorageAuth is only here if we're using AzureAD Authentication but is an Authorizer for Storage
	azureAdStorageAuth *autorest.Authorizer

	accessKey          string
	environment        azure.Environment
	resourceGroupName  string
	storageAccountName string
	sasToken           string
	timeoutSeconds     int
}

func buildArmClient(ctx context.Context, config BackendConfig) (*ArmClient, error) {
	env, err := authentication.AzureEnvironmentByNameFromEndpoint(ctx, config.MetadataHost, config.Environment)
	if err != nil {
		return nil, err
	}

	client := ArmClient{
		environment:        *env,
		resourceGroupName:  config.ResourceGroupName,
		storageAccountName: config.StorageAccountName,
		timeoutSeconds:     config.TimeoutSeconds,
	}

	// if we have an Access Key - we don't need the other clients
	if config.AccessKey != "" {
		client.accessKey = config.AccessKey
		return &client, nil
	}

	// likewise with a SAS token
	if config.SasToken != "" {
		client.sasToken = config.SasToken
		return &client, nil
	}

	authConfig := auth.Credentials{
		ClientID: config.ClientID,
		TenantID: config.TenantID,

		// Service Principal (Client Certificate)
		ClientCertificatePath:     config.ClientCertificatePath,
		ClientCertificatePassword: config.ClientCertificatePassword,

		// Service Principal (Client Secret)
		ClientSecret: config.ClientSecret,

		// Managed Service Identity
		CustomManagedIdentityEndpoint: config.MsiEndpoint,

		// OIDC
		OIDCAssertionToken:          config.OIDCToken,
		GitHubOIDCTokenRequestURL:   config.OIDCRequestURL,
		GitHubOIDCTokenRequestToken: config.OIDCRequestToken,

		AzureCliSubscriptionIDHint: config.SubscriptionID,

		// Feature Toggles
		EnableAuthenticatingUsingClientCertificate: true,
		EnableAuthenticatingUsingClientSecret:      true,
		EnableAuthenticatingUsingAzureCLI:          true,
		EnableAuthenticatingUsingManagedIdentity:   config.UseMsi,
		EnableAuthenticationUsingOIDC:              config.UseOIDC,
		EnableAuthenticationUsingGitHubOIDC:        config.UseOIDC,
	}

	if config.MetadataHost != "" {
		log.Printf("[DEBUG] Configuring cloud environment from Metadata Service at %q", config.MetadataHost)
		authEnvironment, err := environments.FromEndpoint(ctx, fmt.Sprintf("https://%s", config.MetadataHost))
		if err != nil {
			return nil, err
		}
		authConfig.Environment = *authEnvironment
	} else {
		log.Printf("[DEBUG] Configuring built-in cloud environment by name: %q", config.Environment)
		authEnvironment, err := environments.FromName(config.Environment)
		if err != nil {
			return nil, err
		}
		authConfig.Environment = *authEnvironment
	}

	log.Printf("[DEBUG] Obtaining an MSAL / Microsoft Graph token for Resource Manager..")
	authorizer, err := auth.NewAuthorizerFromCredentials(ctx, authConfig, authConfig.Environment.ResourceManager)
	if err != nil {
		return nil, err
	}

	if config.UseAzureADAuthentication {
		log.Printf("[DEBUG] Obtaining an MSAL / Microsoft Graph token for Storage..")
		storageAuth, err := auth.NewAuthorizerFromCredentials(ctx, authConfig, authConfig.Environment.Storage)
		if err != nil {
			return nil, err
		}
		a := autorest.Authorizer(authWrapper.AutorestAuthorizer(storageAuth))
		client.azureAdStorageAuth = &a
	}

	accountsClient := armStorage.NewAccountsClientWithBaseURI(env.ResourceManagerEndpoint, config.SubscriptionID)
	client.configureClient(&accountsClient.Client, authWrapper.AutorestAuthorizer(authorizer))
	client.storageAccountsClient = &accountsClient

	groupsClient := resources.NewGroupsClientWithBaseURI(env.ResourceManagerEndpoint, config.SubscriptionID)
	client.configureClient(&groupsClient.Client, authWrapper.AutorestAuthorizer(authorizer))
	client.groupsClient = &groupsClient

	return &client, nil
}

func (c ArmClient) getBlobClient(ctx context.Context) (*blobs.Client, error) {
	if c.sasToken != "" {
		log.Printf("[DEBUG] Building the Blob Client from a SAS Token")
		storageAuth, err := autorest.NewSASTokenAuthorizer(c.sasToken)
		if err != nil {
			return nil, fmt.Errorf("Error building SAS Token Authorizer: %w", err)
		}

		blobsClient := blobs.NewWithEnvironment(c.environment)
		c.configureClient(&blobsClient.Client, storageAuth)
		return &blobsClient, nil
	}

	if c.azureAdStorageAuth != nil {
		blobsClient := blobs.NewWithEnvironment(c.environment)
		c.configureClient(&blobsClient.Client, *c.azureAdStorageAuth)
		return &blobsClient, nil
	}

	accessKey := c.accessKey
	if accessKey == "" {
		log.Printf("[DEBUG] Building the Blob Client from an Access Token (using user credentials)")
		timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(c.timeoutSeconds)*time.Second)
		defer cancel()
		keys, err := c.storageAccountsClient.ListKeys(timeoutCtx, c.resourceGroupName, c.storageAccountName, "")
		if err != nil {
			return nil, fmt.Errorf("Error retrieving keys for Storage Account %q: %w", c.storageAccountName, err)
		}

		if keys.Keys == nil {
			return nil, fmt.Errorf("Nil key returned for storage account %q", c.storageAccountName)
		}

		accessKeys := *keys.Keys
		accessKey = *accessKeys[0].Value
	}

	storageAuth, err := autorest.NewSharedKeyAuthorizer(c.storageAccountName, accessKey, autorest.SharedKey)
	if err != nil {
		return nil, fmt.Errorf("Error building Shared Key Authorizer: %w", err)
	}

	blobsClient := blobs.NewWithEnvironment(c.environment)
	c.configureClient(&blobsClient.Client, storageAuth)
	return &blobsClient, nil
}

func (c ArmClient) getContainersClient(ctx context.Context) (*containers.Client, error) {
	if c.sasToken != "" {
		log.Printf("[DEBUG] Building the Container Client from a SAS Token")
		storageAuth, err := autorest.NewSASTokenAuthorizer(c.sasToken)
		if err != nil {
			return nil, fmt.Errorf("Error building SAS Token Authorizer: %w", err)
		}

		containersClient := containers.NewWithEnvironment(c.environment)
		c.configureClient(&containersClient.Client, storageAuth)
		return &containersClient, nil
	}

	if c.azureAdStorageAuth != nil {
		containersClient := containers.NewWithEnvironment(c.environment)
		c.configureClient(&containersClient.Client, *c.azureAdStorageAuth)
		return &containersClient, nil
	}

	accessKey := c.accessKey
	if accessKey == "" {
		log.Printf("[DEBUG] Building the Container Client from an Access Token (using user credentials)")
		timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(c.timeoutSeconds)*time.Second)
		defer cancel()
		keys, err := c.storageAccountsClient.ListKeys(timeoutCtx, c.resourceGroupName, c.storageAccountName, "")
		if err != nil {
			return nil, fmt.Errorf("Error retrieving keys for Storage Account %q: %w", c.storageAccountName, err)
		}

		if keys.Keys == nil {
			return nil, fmt.Errorf("Nil key returned for storage account %q", c.storageAccountName)
		}

		accessKeys := *keys.Keys
		accessKey = *accessKeys[0].Value
	}

	storageAuth, err := autorest.NewSharedKeyAuthorizer(c.storageAccountName, accessKey, autorest.SharedKey)
	if err != nil {
		return nil, fmt.Errorf("Error building Shared Key Authorizer: %w", err)
	}

	containersClient := containers.NewWithEnvironment(c.environment)
	c.configureClient(&containersClient.Client, storageAuth)
	return &containersClient, nil
}

func (c *ArmClient) configureClient(client *autorest.Client, auth autorest.Authorizer) {
	client.UserAgent = buildUserAgent()
	client.Authorizer = auth
	client.Sender = buildSender()
	client.SkipResourceProviderRegistration = false
	client.PollingDuration = 60 * time.Minute
}

func buildUserAgent() string {
	userAgent := httpclient.OpenTofuUserAgent(version.Version)

	// append the CloudShell version to the user agent if it exists
	if azureAgent := os.Getenv("AZURE_HTTP_USER_AGENT"); azureAgent != "" {
		userAgent = fmt.Sprintf("%s %s", userAgent, azureAgent)
	}

	return userAgent
}
