// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type AzureCLIAuthConfig struct {
	CLIAuthEnabled bool
}

type azureCLICredentialAuth struct{}

func (cred *azureCLICredentialAuth) Name() string {
	return "Azure CLI Auth"
}

func (cred *azureCLICredentialAuth) Construct(_ context.Context, config *Config) (azcore.TokenCredential, error) {
	// The SubscriptionID and TenantID can be empty, and the logic of this will still be okay
	return azidentity.NewAzureCLICredential(&azidentity.AzureCLICredentialOptions{
		Subscription: config.StorageAddresses.SubscriptionID,
		TenantID:     config.StorageAddresses.TenantID,
	})
}
func (cred *azureCLICredentialAuth) Validate(config *Config) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if !config.CLIAuthEnabled {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Azure Backend: Command Line credentials",
			"Use of command-line auth (az) has been prevented by setting use_cli to false.",
		))
		return diags
	}
	_, err := exec.LookPath("az")
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Azure Backend: Command Line credentials",
			"Error looking for command az in your PATH. Make sure the Azure Command Line tool is installed and executable.",
		))
		return diags
	}
	// Make sure the user is logged in by attempting to get the subscription
	_, err = getCurrentSubscriptionInfo()
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Azure Backend: Command Line credentials",
			"Error using the az command. Make sure you are logged in.",
		))
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
	return checkNamesForAccessKeyCredentials(config.StorageAddresses)
}

type Subscription struct {
	Id        string `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"isDefault"`
}

// getCliAzureSubscriptionID obtains the subscription ID currently active in the
// Azure profile. This assumes the user has the Azure CLI installed on their machine.
func getCliAzureSubscriptionID() (string, error) {
	rawSubscription, err := getCurrentSubscriptionInfo()
	if err != nil {
		return "", err
	}

	var subscription Subscription
	err = json.Unmarshal(rawSubscription, &subscription)
	if err != nil {
		return "", fmt.Errorf("json error for azure subscription: %w", err)
	}

	return subscription.Id, nil
}

// getCurrentSubscriptionInfo is adapted from azure-sdk-for-go's CLI token retrieval
func getCurrentSubscriptionInfo() ([]byte, error) {
	commandLine := "az account show -o json"
	var cliCmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cliCmd = exec.Command("cmd.exe", "/c", commandLine)
	} else {
		cliCmd = exec.Command("/bin/sh", "-c", commandLine)
	}

	stdout, err := cliCmd.Output()
	if errors.Is(err, exec.ErrWaitDelay) && len(stdout) > 0 {
		// The child process wrote to stdout and exited without closing it.
		// Swallow this error and return stdout because it may contain a token.
		return stdout, nil
	}
	if err != nil {
		return nil, err
	}

	return stdout, nil
}
