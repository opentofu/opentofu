// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package r2

import (
	"context"
	"fmt"
	"strings"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/version"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
	"net/http"
)

// New creates a new R2 backend instance
func New(enc encryption.StateEncryption) backend.Backend {
	return &Backend{encryption: enc}
}

// Backend implements the backend.Backend interface for Cloudflare R2
type Backend struct {
	encryption encryption.StateEncryption
	
	// Cloudflare API configuration
	accountID string
	apiToken  string
	
	// R2 bucket configuration
	bucketName         string
	key                string
	workspaceKeyPrefix string
	
	// Optional endpoint override for testing
	endpoint string
	
	// HTTP client for native API calls
	httpClient *http.Client
	
	// Jurisdiction for bucket operations
	jurisdiction string
}

// ConfigSchema returns the configuration schema for the R2 backend
func (b *Backend) ConfigSchema() *configschema.Block {
	return &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"account_id": {
				Type:        cty.String,
				Required:    true,
				Description: "The Cloudflare account ID",
			},
			"api_token": {
				Type:        cty.String,
				Required:    true,
				Sensitive:   true,
				Description: "The Cloudflare API token with R2 permissions",
			},
			"bucket": {
				Type:        cty.String,
				Required:    true,
				Description: "The name of the R2 bucket",
			},
			"key": {
				Type:        cty.String,
				Required:    true,
				Description: "The path to the state file inside the bucket",
			},
			"workspace_key_prefix": {
				Type:        cty.String,
				Optional:    true,
				Description: "The prefix applied to the state path inside the bucket. Default: env:",
			},
			"endpoint": {
				Type:        cty.String,
				Optional:    true,
				Description: "Custom endpoint URL (primarily for testing)",
			},
			"jurisdiction": {
				Type:        cty.String,
				Optional:    true,
				Description: "The jurisdiction for the R2 bucket (e.g., 'eu' for European Union)",
			},
		},
	}
}

// PrepareConfig validates and prepares the backend configuration
func (b *Backend) PrepareConfig(configVal cty.Value) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	
	if configVal.IsNull() {
		return configVal, diags
	}
	
	// Validate account ID format
	if accountID := configVal.GetAttr("account_id"); !accountID.IsNull() {
		id := accountID.AsString()
		if len(id) != 32 || !isHexadecimal(id) {
			diags = diags.Append(tfdiags.AttributeValue(
				tfdiags.Error,
				"Invalid account ID",
				"The account_id must be a 32-character hexadecimal string",
				cty.Path{cty.GetAttrStep{Name: "account_id"}},
			))
		}
	}
	
	// Validate bucket name
	if bucket := configVal.GetAttr("bucket"); !bucket.IsNull() {
		name := bucket.AsString()
		if err := validateBucketName(name); err != nil {
			diags = diags.Append(tfdiags.AttributeValue(
				tfdiags.Error,
				"Invalid bucket name",
				err.Error(),
				cty.Path{cty.GetAttrStep{Name: "bucket"}},
			))
		}
	}
	
	// Validate key format
	if key := configVal.GetAttr("key"); !key.IsNull() {
		k := key.AsString()
		if err := validateKey(k); err != nil {
			diags = diags.Append(tfdiags.AttributeValue(
				tfdiags.Error,
				"Invalid key",
				err.Error(),
				cty.Path{cty.GetAttrStep{Name: "key"}},
			))
		}
	}
	
	// Validate jurisdiction if provided
	if jurisdiction := configVal.GetAttr("jurisdiction"); !jurisdiction.IsNull() {
		j := jurisdiction.AsString()
		if j != "" && j != "eu" && j != "fedramp" {
			diags = diags.Append(tfdiags.AttributeValue(
				tfdiags.Error,
				"Invalid jurisdiction",
				"The jurisdiction must be either 'eu' or 'fedramp'",
				cty.Path{cty.GetAttrStep{Name: "jurisdiction"}},
			))
		}
	}
	
	return configVal, diags
}

// Configure initializes the backend with the provided configuration
func (b *Backend) Configure(ctx context.Context, configVal cty.Value) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	
	// Extract configuration values
	data := &schema{
		WorkspaceKeyPrefix: "env:",
	}
	
	if err := gocty.FromCtyValue(configVal, data); err != nil {
		diags = diags.Append(err)
		return diags
	}
	
	// Set backend fields
	b.accountID = data.AccountID
	b.apiToken = data.APIToken
	b.bucketName = data.Bucket
	b.key = data.Key
	b.workspaceKeyPrefix = data.WorkspaceKeyPrefix
	b.endpoint = data.Endpoint
	b.jurisdiction = data.Jurisdiction
	
	// Initialize HTTP client with user agent
	b.httpClient = httpclient.New(ctx)
	
	// Verify bucket exists and is accessible
	if err := b.verifyBucketAccess(ctx); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to verify R2 bucket access",
			fmt.Sprintf("Error verifying access to bucket %q: %s", b.bucketName, err),
		))
	}
	
	return diags
}

// schema represents the configuration structure
type schema struct {
	AccountID          string `cty:"account_id"`
	APIToken           string `cty:"api_token"`
	Bucket             string `cty:"bucket"`
	Key                string `cty:"key"`
	WorkspaceKeyPrefix string `cty:"workspace_key_prefix"`
	Endpoint           string `cty:"endpoint"`
	Jurisdiction       string `cty:"jurisdiction"`
}

// isHexadecimal checks if a string contains only hexadecimal characters
func isHexadecimal(s string) bool {
	for _, c := range s {
		if c < '0' || (c > '9' && c < 'A') || (c > 'F' && c < 'a') || c > 'f' {
			return false
		}
	}
	return true
}

// validateBucketName validates R2 bucket naming rules
func validateBucketName(name string) error {
	if len(name) < 3 || len(name) > 63 {
		return fmt.Errorf("bucket name must be between 3 and 63 characters")
	}
	
	// Check for uppercase letters first
	for _, c := range name {
		if c >= 'A' && c <= 'Z' {
			return fmt.Errorf("bucket name can only contain lowercase letters, numbers, and dashes")
		}
	}
	
	if !isValidBucketChar(rune(name[0])) || !isValidBucketChar(rune(name[len(name)-1])) {
		return fmt.Errorf("bucket name must start and end with a letter or number")
	}
	
	prevDash := false
	for _, c := range name {
		if c == '-' {
			if prevDash {
				return fmt.Errorf("bucket name cannot contain consecutive dashes")
			}
			prevDash = true
		} else {
			prevDash = false
			if !isValidBucketChar(c) && c != '-' {
				return fmt.Errorf("bucket name can only contain lowercase letters, numbers, and dashes")
			}
		}
	}
	
	if strings.Contains(name, "..") {
		return fmt.Errorf("bucket name cannot contain '..'")
	}
	
	return nil
}

// isValidBucketChar checks if a character is valid for bucket names
func isValidBucketChar(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
}

// validateKey validates the state key format
func validateKey(key string) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}
	
	if strings.HasPrefix(key, "/") {
		return fmt.Errorf("key cannot start with '/'")
	}
	
	if strings.HasSuffix(key, "/") {
		return fmt.Errorf("key cannot end with '/'")
	}
	
	// Check for invalid characters
	for _, c := range key {
		if c < 32 || c > 126 {
			return fmt.Errorf("key contains invalid characters")
		}
	}
	
	return nil
}

// verifyBucketAccess checks if the bucket exists and is accessible
func (b *Backend) verifyBucketAccess(ctx context.Context) error {
	// Get bucket information using native Cloudflare API
	url := b.getAPIEndpoint() + fmt.Sprintf("/accounts/%s/r2/buckets/%s", b.accountID, b.bucketName)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	
	req.Header.Set("Authorization", "Bearer "+b.apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", httpclient.OpenTofuUserAgent(version.String()))
	
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == 404 {
		return fmt.Errorf("bucket not found")
	}
	
	if resp.StatusCode != 200 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	
	return nil
}

// getAPIEndpoint returns the Cloudflare API endpoint
func (b *Backend) getAPIEndpoint() string {
	if b.endpoint != "" {
		return b.endpoint
	}
	return "https://api.cloudflare.com/client/v4"
}

// getR2Endpoint returns the R2 S3-compatible endpoint for object operations
func (b *Backend) getR2Endpoint() string {
	if b.endpoint != "" {
		// For testing, use the provided endpoint
		return b.endpoint
	}
	
	// Build the R2 endpoint based on jurisdiction
	switch b.jurisdiction {
	case "eu":
		return fmt.Sprintf("https://%s.eu.r2.cloudflarestorage.com", b.accountID)
	case "fedramp":
		return fmt.Sprintf("https://%s.fedramp.r2.cloudflarestorage.com", b.accountID)
	default:
		return fmt.Sprintf("https://%s.r2.cloudflarestorage.com", b.accountID)
	}
}
