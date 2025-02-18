// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package aws_kms

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
	awsbase "github.com/hashicorp/aws-sdk-go-base/v2"
	baselogging "github.com/hashicorp/aws-sdk-go-base/v2/logging"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/version"
)

// Can be overridden for test mocking
var newKMSFromConfig func(aws.Config) kmsClient = func(cfg aws.Config) kmsClient {
	return kms.NewFromConfig(cfg)
}

type Config struct {
	// KeyProvider Config
	KMSKeyID string `hcl:"kms_key_id"`
	KeySpec  string `hcl:"key_spec"`

	// Mirrored S3 Backend Config, mirror any changes
	AccessKey                      string                     `hcl:"access_key,optional"`
	Endpoints                      []ConfigEndpoints          `hcl:"endpoints,block"`
	MaxRetries                     int                        `hcl:"max_retries,optional"`
	Profile                        string                     `hcl:"profile,optional"`
	Region                         string                     `hcl:"region,optional"`
	SecretKey                      string                     `hcl:"secret_key,optional"`
	SkipCredsValidation            bool                       `hcl:"skip_credentials_validation,optional"`
	SkipRequestingAccountId        bool                       `hcl:"skip_requesting_account_id,optional"`
	STSRegion                      string                     `hcl:"sts_region,optional"`
	Token                          string                     `hcl:"token,optional"`
	HTTPProxy                      *string                    `hcl:"http_proxy,optional"`
	HTTPSProxy                     *string                    `hcl:"https_proxy,optional"`
	NoProxy                        string                     `hcl:"no_proxy,optional"`
	Insecure                       bool                       `hcl:"insecure,optional"`
	UseDualStackEndpoint           bool                       `hcl:"use_dualstack_endpoint,optional"`
	UseFIPSEndpoint                bool                       `hcl:"use_fips_endpoint,optional"`
	CustomCABundle                 string                     `hcl:"custom_ca_bundle,optional"`
	EC2MetadataServiceEndpoint     string                     `hcl:"ec2_metadata_service_endpoint,optional"`
	EC2MetadataServiceEndpointMode string                     `hcl:"ec2_metadata_service_endpoint_mode,optional"`
	SkipMetadataAPICheck           *bool                      `hcl:"skip_metadata_api_check,optional"`
	SharedCredentialsFiles         []string                   `hcl:"shared_credentials_files,optional"`
	SharedConfigFiles              []string                   `hcl:"shared_config_files,optional"`
	AssumeRole                     *AssumeRole                `hcl:"assume_role,optional"`
	AssumeRoleWithWebIdentity      *AssumeRoleWithWebIdentity `hcl:"assume_role_with_web_identity,optional"`
	AllowedAccountIds              []string                   `hcl:"allowed_account_ids,optional"`
	ForbiddenAccountIds            []string                   `hcl:"forbidden_account_ids,optional"`
	RetryMode                      string                     `hcl:"retry_mode,optional"`
}

func stringAttrEnvFallback(val string, env string) string {
	if val != "" {
		return val
	}
	return os.Getenv(env)
}

func stringArrayAttrEnvFallback(val []string, env string) []string {
	if len(val) != 0 {
		return val
	}
	envVal := os.Getenv(env)
	if envVal != "" {
		return []string{envVal}
	}
	return nil
}

func (c Config) asAWSBase() (*awsbase.Config, error) {
	// Get endpoints to use
	endpoints, err := c.getEndpoints()
	if err != nil {
		return nil, err
	}

	// Get assume role
	assumeRole, err := c.AssumeRole.asAWSBase()
	if err != nil {
		return nil, err
	}
	var roles []awsbase.AssumeRole
	if assumeRole != nil {
		roles = append(roles, *assumeRole)
	}

	// Get assume role with web identity
	assumeRoleWithWebIdentity, err := c.AssumeRoleWithWebIdentity.asAWSBase()
	if err != nil {
		return nil, err
	}

	// validate region
	if c.Region == "" && os.Getenv("AWS_REGION") == "" && os.Getenv("AWS_DEFAULT_REGION") == "" {
		return nil, fmt.Errorf(`the "region" attribute or the "AWS_REGION" or "AWS_DEFAULT_REGION" environment variables must be set.`)
	}

	// Retry Mode
	if c.MaxRetries == 0 {
		c.MaxRetries = 5
	}
	var retryMode aws.RetryMode
	if len(c.RetryMode) != 0 {
		retryMode, err = aws.ParseRetryMode(c.RetryMode)
		if err != nil {
			return nil, fmt.Errorf("%w: expected %q or %q", err, aws.RetryModeStandard, aws.RetryModeAdaptive)
		}
	}

	// IDMS handling
	imdsEnabled := imds.ClientDefaultEnableState
	if c.SkipMetadataAPICheck != nil {
		if *c.SkipMetadataAPICheck {
			imdsEnabled = imds.ClientEnabled
		} else {
			imdsEnabled = imds.ClientDisabled
		}
	}

	// validate account_ids
	if len(c.AllowedAccountIds) != 0 && len(c.ForbiddenAccountIds) != 0 {
		return nil, fmt.Errorf("conflicting config attributes: only allowed_account_ids or forbidden_account_ids can be specified, not both")
	}

	return &awsbase.Config{
		AccessKey:               c.AccessKey,
		CallerDocumentationURL:  "https://opentofu.org/docs/language/settings/backends/s3", // TODO
		CallerName:              "KMS Key Provider",
		IamEndpoint:             stringAttrEnvFallback(endpoints.IAM, "AWS_ENDPOINT_URL_IAM"),
		MaxRetries:              c.MaxRetries,
		RetryMode:               retryMode,
		Profile:                 c.Profile,
		Region:                  c.Region,
		SecretKey:               c.SecretKey,
		SkipCredsValidation:     c.SkipCredsValidation,
		SkipRequestingAccountId: c.SkipRequestingAccountId,
		StsEndpoint:             stringAttrEnvFallback(endpoints.STS, "AWS_ENDPOINT_URL_STS"),
		StsRegion:               c.STSRegion,
		Token:                   c.Token,

		// Note: we don't need to read env variables explicitly because they are read implicitly by aws-sdk-base-go:
		// see: https://github.com/hashicorp/aws-sdk-go-base/blob/v2.0.0-beta.41/internal/config/config.go#L133
		// which relies on: https://cs.opensource.google/go/x/net/+/refs/tags/v0.18.0:http/httpproxy/proxy.go;l=89-96
		HTTPProxy:            c.HTTPProxy,
		HTTPSProxy:           c.HTTPSProxy,
		NoProxy:              c.NoProxy,
		Insecure:             c.Insecure,
		UseDualStackEndpoint: c.UseDualStackEndpoint,
		UseFIPSEndpoint:      c.UseFIPSEndpoint,
		UserAgent: awsbase.UserAgentProducts{
			{Name: "APN", Version: "1.0"},
			{Name: httpclient.DefaultApplicationName, Version: version.String()},
		},
		CustomCABundle: stringAttrEnvFallback(c.CustomCABundle, "AWS_CA_BUNDLE"),

		EC2MetadataServiceEnableState:  imdsEnabled,
		EC2MetadataServiceEndpoint:     stringAttrEnvFallback(c.EC2MetadataServiceEndpoint, "AWS_EC2_METADATA_SERVICE_ENDPOINT"),
		EC2MetadataServiceEndpointMode: stringAttrEnvFallback(c.EC2MetadataServiceEndpointMode, "AWS_EC2_METADATA_SERVICE_ENDPOINT_MODE"),

		SharedCredentialsFiles:    stringArrayAttrEnvFallback(c.SharedCredentialsFiles, "AWS_SHARED_CREDENTIALS_FILE"),
		SharedConfigFiles:         stringArrayAttrEnvFallback(c.SharedConfigFiles, "AWS_SHARED_CONFIG_FILE"),
		AssumeRole:                roles,
		AssumeRoleWithWebIdentity: assumeRoleWithWebIdentity,
		AllowedAccountIds:         c.AllowedAccountIds,
		ForbiddenAccountIds:       c.ForbiddenAccountIds,
	}, nil
}

func (c Config) Build() (keyprovider.KeyProvider, keyprovider.KeyMeta, error) {
	err := c.validate()
	if err != nil {
		return nil, nil, err
	}

	cfg, err := c.asAWSBase()
	if err != nil {
		return nil, nil, err
	}

	ctx := context.Background()
	ctx, baselog := attachLoggerToContext(ctx)
	cfg.Logger = baselog

	_, awsConfig, awsDiags := awsbase.GetAwsConfig(ctx, cfg)

	if awsDiags.HasError() {
		out := "errors were encountered in aws kms configuration"
		for _, diag := range awsDiags.Errors() {
			out += "\n" + diag.Summary() + " : " + diag.Detail()
		}

		return nil, nil, fmt.Errorf(out)
	}

	return &keyProvider{
		Config: c,
		svc:    newKMSFromConfig(awsConfig),
		ctx:    ctx,
	}, new(keyMeta), nil
}

// validate checks the configuration for the key provider
func (c Config) validate() (err error) {
	if c.KMSKeyID == "" {
		return &keyprovider.ErrInvalidConfiguration{
			Message: "no kms_key_id provided",
		}
	}

	if c.KeySpec == "" {
		return &keyprovider.ErrInvalidConfiguration{
			Message: "no key_spec provided",
		}
	}

	spec := c.getKeySpecAsAWSType()
	if spec == nil {
		// This is to fetch a list of the values from the enum, because `spec` here can be nil, so we have to grab
		// at least one of the enum possibilities here just to call .Values()
		values := types.DataKeySpecAes256.Values()
		return &keyprovider.ErrInvalidConfiguration{
			Message: fmt.Sprintf("invalid key_spec %s, expected one of %v", c.KeySpec, values),
		}
	}

	return nil
}

// getSpecAsAWSType handles conversion between the string from the config and the aws expected enum type
// it will return nil if it cannot find a match
func (c Config) getKeySpecAsAWSType() *types.DataKeySpec {
	var spec types.DataKeySpec
	for _, opt := range spec.Values() {
		if string(opt) == c.KeySpec {
			return &opt
		}
	}
	return nil
}

// Mirrored from s3 backend config
func attachLoggerToContext(ctx context.Context) (context.Context, baselogging.HcLogger) {
	ctx, baseLog := baselogging.NewHcLogger(ctx, logging.HCLogger().Named("backend-s3"))
	ctx = baselogging.RegisterLogger(ctx, baseLog)
	return ctx, baseLog
}
