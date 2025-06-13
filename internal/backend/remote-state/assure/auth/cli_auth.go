package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type AzureCLIAuthConfig struct {
	CLIAuthDisabled bool
}

type azureCLICredentialAuth struct{}

func (cred *azureCLICredentialAuth) Construct(_ context.Context, config *Config) (azcore.TokenCredential, error) {
	// The SubscriptionID and TenantID can be empty, and the logic of this will still be okay
	return azidentity.NewAzureCLICredential(&azidentity.AzureCLICredentialOptions{
		Subscription: config.StorageAddresses.SubscriptionID,
		TenantID:     config.StorageAddresses.TenantID,
	})
}
func (cred *azureCLICredentialAuth) Validate(config *Config) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if config.CLIAuthDisabled {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"CLI Auth is explicitly disabled",
			"You set disable_cli to true, which prevents the CLI Auth from being used. Remove this unless you're testing internals or otherwise explicitly want to prevent CLI Authentication.",
		))
		return diags
	}
	// We'll try constructing it and getting a token
	tempAuth, err := cred.Construct(context.Background(), config)
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Error constructing test CLI credentials",
			fmt.Sprintf("CLI credentials encountered an error while initalizing: %s", err.Error()),
		))
		return diags
	}
	_, err = tempAuth.GetToken(context.Background(), policy.TokenRequestOptions{Scopes: []string{"https://management.core.windows.net/.default"}})
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Error constructing test CLI token",
			fmt.Sprintf("CLI credentials encountered an error while attempting to make a test token: %s", err.Error()),
		))
		return diags
	}
	return diags
}

func (cred *azureCLICredentialAuth) AugmentConfig(config *Config) (err error) {
	if config.StorageAddresses.SubscriptionID == "" {
		config.StorageAddresses.SubscriptionID, err = getCliAzureSubscriptionID()
		if err != nil {
			return err
		}
	}
	return checkNamesForAccessKeyCredentials(*config.StorageAddresses)
}

type Subscription struct {
	Id        string `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"isDefault"`
}

type Profile struct {
	Subscriptions []Subscription `json:"subscriptions"`
}

// getCliAzureSubscriptionID obtains the subscription ID currently active in the
// Azure profile. This assumes the user has an Azure profile saved to their
// home directory, which is usually provided by the Azure command line tool when
// using `az login`.
//
// # TODO make sure this is compatible with Windows
//
// # TODO do we want to obtain anything else from the profile setting? Tenant ID, perhaps?
func getCliAzureSubscriptionID() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	azureProfileFilePath := filepath.Join(home, ".azure", "azureProfile.json")
	rawFile, err := os.ReadFile(azureProfileFilePath)
	if err != nil {
		return "", fmt.Errorf("error reading azure profile at %s: %w", azureProfileFilePath, err)
	}
	// Trim BOM
	rawFile = bytes.TrimPrefix(rawFile, []byte("\xef\xbb\xbf"))

	var profile Profile
	err = json.Unmarshal(rawFile, &profile)
	if err != nil {
		return "", fmt.Errorf("json error for azure profile at %s: %w", azureProfileFilePath, err)
	}

	for _, sub := range profile.Subscriptions {
		if sub.IsDefault {
			return sub.Id, nil
		}
	}

	return "", fmt.Errorf("no default subscription found in azureProfile.json")
}
