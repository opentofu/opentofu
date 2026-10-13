// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

// Package oci stores state in Oracle Cloud Infrastructure Object Storage.
package oci

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/objectstorage"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/version"
)

// The SDK has a process-wide logger. Even its redacted verbose output can
// contain raw state or unrecognized token payloads. Providers run separately;
// this process uses the SDK only for backend operations, which never log bodies.
type silentSDKLogger struct{}

func (silentSDKLogger) LogLevel() int                         { return 0 }
func (silentSDKLogger) Log(int, string, ...interface{}) error { return nil }
func init()                                                   { common.SetSDKLogger(silentSDKLogger{}) }

type Backend struct {
	encryption                                               encryption.StateEncryption
	storageClient                                            objectStorageClient
	namespace, bucketName, key, workspaceKeyPrefix, kmsKeyID string
	requestTimeout                                           time.Duration
	mu                                                       sync.Mutex
	managers                                                 map[string]*remote.State

	// An instance-local seam keeps authentication/network tests independent.
	newClient func(context.Context, authConfig, time.Duration) (objectStorageClient, error)
}

var _ backend.Backend = (*Backend)(nil)

func New(enc encryption.StateEncryption) backend.Backend {
	return &Backend{encryption: enc, newClient: newStorageClient}
}

var authAttributes = map[string]string{
	"auth":                                "OCI authentication mode (defaults to ApiKey).",
	"tenancy_ocid":                        "Tenancy OCID for API key authentication.",
	"user_ocid":                           "User OCID for API key authentication.",
	"fingerprint":                         "API signing key fingerprint.",
	"private_key":                         "PEM API signing private key.",
	"private_key_path":                    "Path to the API signing private key.",
	"private_key_password":                "Passphrase for the API signing private key.",
	"region":                              "Object Storage bucket region.",
	"config_file":                         "OCI SDK configuration file path.",
	"config_file_profile":                 "Named OCI configuration profile (defaults to DEFAULT).",
	"certificates_directory":              "Directory containing instance principal certificates and key.",
	"workload_identity_token_path":        "Path to the external workload identity token.",
	"token_exchange_domain_url":           "Identity domain URL for workload token exchange.",
	"token_exchange_auth":                 "Token exchange authentication method.",
	"token_exchange_client_id":            "OAuth client ID for workload token exchange.",
	"token_exchange_client_secret":        "OAuth client secret for workload token exchange.",
	"token_exchange_requested_token_type": "Requested workload federation token type.",
	"token_exchange_subject_token_type":   "Subject workload federation token type.",
	"token_exchange_resource_type":        "Workload federation resource type.",
	"token_exchange_rpst_exp":             "Requested resource principal token lifetime in minutes.",
	"token_exchange_public_key":           "Workload federation public key.",
}

func (b *Backend) ConfigSchema() *configschema.Block {
	attrs := map[string]*configschema.Attribute{
		"namespace":            {Type: cty.String, Required: true, Description: "Object Storage namespace containing the bucket."},
		"bucket":               {Type: cty.String, Required: true, Description: "Existing Object Storage bucket name."},
		"key":                  {Type: cty.String, Optional: true, Description: "State object name (defaults to terraform.tfstate)."},
		"workspace_key_prefix": {Type: cty.String, Optional: true, Description: "Prefix for named workspace state (defaults to env:)."},
		"kms_key_id":           {Type: cty.String, Optional: true, Description: "Optional OCI Vault key OCID for state and lock encryption."},
		"request_timeout":      {Type: cty.String, Optional: true, Description: "Object Storage operation and HTTP request timeout (defaults to 60s)."},
	}
	for name, description := range authAttributes {
		attrs[name] = &configschema.Attribute{Type: cty.String, Optional: true, Description: description,
			Sensitive: name == "private_key" || name == "private_key_password" || name == "token_exchange_client_secret"}
	}
	return &configschema.Block{Attributes: attrs}
}

func configString(config cty.Value, name string) string {
	value := config.GetAttr(name)
	if value.IsNull() || !value.IsKnown() {
		return ""
	}
	return value.AsString()
}

func (b *Backend) PrepareConfig(config cty.Value) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	config, err := b.ConfigSchema().CoerceValue(config)
	if err != nil {
		return config, diags.Append(err)
	}
	values := config.AsValueMap()
	for name, fallback := range map[string]string{"key": "terraform.tfstate", "workspace_key_prefix": "env:", "request_timeout": "60s"} {
		if values[name].IsNull() {
			values[name] = cty.StringVal(fallback)
		}
	}
	config = cty.ObjectVal(values)
	invalid := func(name, detail string) {
		diags = diags.Append(tfdiags.AttributeValue(tfdiags.Error, "Invalid oracle-oci backend configuration", detail, cty.GetAttrPath(name)))
	}
	for name, value := range values {
		if !value.IsKnown() {
			invalid(name, "Backend configuration values must be known during initialization.")
		}
	}
	for _, name := range []string{"namespace", "bucket"} {
		value := configString(config, name)
		if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "/\\") || strings.IndexFunc(value, unicode.IsControl) >= 0 {
			invalid(name, name+" must be a nonempty name without slashes or control characters.")
		}
	}
	for _, name := range []string{"key", "workspace_key_prefix"} {
		value := configString(config, name)
		if (name == "key" || value != "") && !validObjectPath(value) {
			invalid(name, name+" must be a relative object path without empty, '.' or '..' components, backslashes, or control characters.")
		}
	}
	if mode := configString(config, "auth"); mode != "" {
		valid := false
		for _, allowed := range supportedAuthModes {
			valid = valid || strings.EqualFold(mode, allowed)
		}
		if !valid {
			invalid("auth", "Unsupported authentication mode; see the oracle-oci backend authentication matrix.")
		}
	}
	if duration, err := time.ParseDuration(configString(config, "request_timeout")); err != nil || duration <= 0 {
		invalid("request_timeout", "request_timeout must be a positive duration, such as 60s.")
	}
	return config, diags
}

func validObjectPath(value string) bool {
	if value == "" || strings.Contains(value, "\\") || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func (b *Backend) Configure(ctx context.Context, config cty.Value) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if err := ctx.Err(); err != nil {
		return diags.Append(err)
	}
	cfg := authConfig{
		Mode: configString(config, "auth"), TenancyOCID: configString(config, "tenancy_ocid"),
		UserOCID: configString(config, "user_ocid"), Fingerprint: configString(config, "fingerprint"),
		PrivateKey: configString(config, "private_key"), PrivateKeyPath: configString(config, "private_key_path"),
		PrivateKeyPassword: configString(config, "private_key_password"), Region: configString(config, "region"),
		ConfigFile: configString(config, "config_file"), ConfigFileProfile: configString(config, "config_file_profile"),
		CertificatesDirectory:     configString(config, "certificates_directory"),
		WorkloadIdentityTokenPath: configString(config, "workload_identity_token_path"),
		TokenExchangeDomainURL:    configString(config, "token_exchange_domain_url"), TokenExchangeAuth: configString(config, "token_exchange_auth"),
		TokenExchangeClientID: configString(config, "token_exchange_client_id"), TokenExchangeClientSecret: configString(config, "token_exchange_client_secret"),
		TokenExchangeRequestedTokenType: configString(config, "token_exchange_requested_token_type"),
		TokenExchangeSubjectTokenType:   configString(config, "token_exchange_subject_token_type"), TokenExchangeResourceType: configString(config, "token_exchange_resource_type"),
		TokenExchangeRPSTExp: configString(config, "token_exchange_rpst_exp"), TokenExchangePublicKey: configString(config, "token_exchange_public_key"),
	}
	timeout, err := time.ParseDuration(configString(config, "request_timeout"))
	if err != nil || timeout <= 0 {
		return diags.Append(fmt.Errorf("OCI request_timeout must be a positive duration"))
	}
	factory := b.newClient
	if factory == nil {
		factory = newStorageClient
	}
	client, err := factory(ctx, cfg, timeout)
	if err != nil {
		return diags.Append(err)
	}
	if err := ctx.Err(); err != nil {
		return diags.Append(err)
	}
	b.storageClient = client
	b.namespace, b.bucketName = configString(config, "namespace"), configString(config, "bucket")
	b.key, b.workspaceKeyPrefix = configString(config, "key"), configString(config, "workspace_key_prefix")
	b.kmsKeyID, b.requestTimeout = configString(config, "kms_key_id"), timeout
	b.managers = make(map[string]*remote.State)
	return diags
}

func newStorageClient(ctx context.Context, cfg authConfig, timeout time.Duration) (objectStorageClient, error) {
	provider, err := buildConfigurationProvider(cfg)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	client, err := objectstorage.NewObjectStorageClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, fmt.Errorf("cannot initialize OCI Object Storage client; verify authentication and bucket region")
	}
	// Retain the SDK's transport, including its certificate configuration.
	if transport, ok := client.HTTPClient.(*http.Client); ok {
		transport.Timeout = timeout
	}
	client.UserAgent += " OpenTofu/" + version.Version
	if err := configureDelegation(&client, provider); err != nil {
		return nil, err
	}
	headers := common.DefaultGenericHeaders()
	if client.Interceptor != nil {
		headers = append(headers, delegationHeader)
	}
	return &serializedObjectStorageClient{
		client: &client,
		newClient: func() objectStorageClient {
			// SDK retries may continue in their own goroutine after cancellation.
			// Give each operation its own signer snapshot, so a late signer can
			// never overwrite the token/key pair of a subsequent operation.
			clone := client
			clone.Signer = common.RequestSigner(newSnapshotSigningProvider(provider), headers, common.DefaultBodyHeaders())
			return &clone
		},
	}, nil
}
