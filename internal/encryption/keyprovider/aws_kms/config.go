package aws_kms

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	awsbase "github.com/hashicorp/aws-sdk-go-base/v2"
	baselogging "github.com/hashicorp/aws-sdk-go-base/v2/logging"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/version"
)

type Config struct {
	// KeyProvider Config
	KMSKeyID string `hcl:"kms_key_id" json:"-"`
	KeySpec  string `hcl:"key_spec" json:"-"`
	Symetric bool   `hcl:"symetric" json:"-"`

	// Mirrored Backend Config
	AccessKey                      string                     `hcl:"access_key,optional" json:"-"`
	Endpoints                      []ConfigEndpoints          `hcl:"endpoints,block" json:"-"`
	MaxRetries                     int                        `hcl:"max_retries,optional" json:"-"`
	Profile                        string                     `hcl:"profile,optional" json:"-"`
	Region                         string                     `hcl:"region,optional" json:"-"`
	SecretKey                      string                     `hcl:"secret_key,optional" json:"-"`
	SkipCredsValidation            bool                       `hcl:"skip_credentials_validation,optional" json:"-"`
	SkipRequestingAccountId        bool                       `hcl:"skip_requesting_account_id,optional" json:"-"`
	STSRegion                      string                     `hcl:"sts_region,optional" json:"-"`
	Token                          string                     `hcl:"token,optional" json:"-"`
	HTTPProxy                      *string                    `hcl:"http_proxy,optional" json:"-"`
	HTTPSProxy                     *string                    `hcl:"https_proxy,optional" json:"-"`
	NoProxy                        string                     `hcl:"no_proxy,optional" json:"-"`
	Insecure                       bool                       `hcl:"insecure,optional" json:"-"`
	UseDualStackEndpoint           bool                       `hcl:"use_dualstack_endpoint,optional" json:"-"`
	UseFIPSEndpoint                bool                       `hcl:"use_fips_endpoint,optional" json:"-"`
	CustomCABundle                 string                     `hcl:"custom_ca_bundle,optional" json:"-"`
	EC2MetadataServiceEndpoint     string                     `hcl:"ec2_metadata_service_endpoint,optional" json:"-"`
	EC2MetadataServiceEndpointMode string                     `hcl:"ec2_metadata_service_endpoint_mode,optional" json:"-"`
	SkipMetadataAPICheck           *bool                      `hcl:"skip_metadata_api_check,optional" json:"-"`
	SharedCredentialsFile          string                     `hcl:"shared_credentials_file,optional" json:"-"`
	SharedCredentialsFiles         []string                   `hcl:"shared_credentials_files,optional" json:"-"`
	SharedConfigFiles              []string                   `hcl:"shared_config_files,optional" json:"-"`
	AssumeRole                     *AssumeRole                `hcl:"assume_role,optional" json:"-"`
	AssumeRoleWithWebIdentity      *AssumeRoleWithWebIdentity `hcl:"assume_role_with_web_identity,optional" json:"-"`
	AllowedAccountIds              []string                   `hcl:"allowed_account_ids,optional" json:"-"`
	ForbiddenAccountIds            []string                   `hcl:"forbidden_account_ids,optional" json:"-"`
	RetryMode                      string                     `hcl:"retry_mode,optional" json:"-"`
}

type ConfigEndpoints struct {
	IAM string `hcl:"iam,optional"`
	STS string `hcl:"sts,optional"`
}

type AssumeRole struct {
	RoleARN           string            `hcl:"role_arn"`
	Duration          string            `hcl:"duration,optional"`
	ExternalID        string            `hcl:"external_id,optional"`
	Policy            string            `hcl:"policy,optional"`
	PolicyARNs        []string          `hcl:"policy_arns,optional"`
	SessionName       string            `hcl:"session_name,optional"`
	Tags              map[string]string `hcl:"tags,optional"`
	TransitiveTagKeys []string          `hcl:"transitive_tag_keys,optional"`
}

type AssumeRoleWithWebIdentity struct {
	RoleARN              string   `hcl:"role_arn,optional"`
	Duration             string   `hcl:"duration,optional"`
	Policy               string   `hcl:"policy,optional"`
	PolicyARNs           []string `hcl:"policy_arns,optional"`
	SessionName          string   `hcl:"session_name,optional"`
	WebIdentityToken     string   `hcl:"web_identity_token,optional"`
	WebIdentityTokenFile string   `hcl:"web_identity_token_file,optional"`
}

func (c Config) Build() (keyprovider.KeyProvider, keyprovider.KeyMeta, error) {
	ctx := context.TODO()
	ctx, baselog := attachLoggerToContext(ctx)

	endpoints := ConfigEndpoints{}
	if len(c.Endpoints) == 1 {
		endpoints = c.Endpoints[1]
	}
	if len(c.Endpoints) > 1 {
		return nil, nil, fmt.Errorf("expected single aws_kms endpoints block, multiple provided")
	}

	// env var fallbacks
	if len(endpoints.IAM) == 0 {
		endpoints.IAM = os.Getenv("AWS_ENDPOINT_URL_IAM")
	}
	if len(endpoints.STS) == 0 {
		endpoints.STS = os.Getenv("AWS_ENDPOINT_URL_STS")
	}
	if len(c.CustomCABundle) == 0 {
		c.CustomCABundle = os.Getenv("AWS_CA_BUNDLE")
	}
	if len(c.EC2MetadataServiceEndpoint) == 0 {
		c.EC2MetadataServiceEndpoint = os.Getenv("AWS_EC2_METADATA_SERVICE_ENDPOINT")
	}
	if len(c.EC2MetadataServiceEndpointMode) == 0 {
		c.EC2MetadataServiceEndpointMode = os.Getenv("AWS_EC2_METADATA_SERVICE_ENDPOINT_MODE")
	}

	// Endpoint formatting
	if len(endpoints.IAM) != 0 {
		endpoints.IAM = includeProtoIfNessesary(endpoints.IAM)
	}
	if len(endpoints.STS) != 0 {
		endpoints.STS = includeProtoIfNessesary(endpoints.STS)
	}

	// Defaults
	if c.MaxRetries == 0 {
		c.MaxRetries = 5
	}

	// TODO include the full validation done by the s3 backend, which this is based off of

	cfg := &awsbase.Config{
		AccessKey:               c.AccessKey,
		CallerDocumentationURL:  "https://opentofu.org/docs/language/settings/backends/s3", // TODO
		CallerName:              "KMS Key Provider",
		IamEndpoint:             endpoints.IAM,
		MaxRetries:              c.MaxRetries,
		Profile:                 c.Profile,
		Region:                  c.Region,
		SecretKey:               c.SecretKey,
		SkipCredsValidation:     c.SkipCredsValidation,
		SkipRequestingAccountId: c.SkipRequestingAccountId,
		StsEndpoint:             endpoints.STS,
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
		CustomCABundle:                 c.CustomCABundle,
		EC2MetadataServiceEndpoint:     c.EC2MetadataServiceEndpoint,
		EC2MetadataServiceEndpointMode: c.EC2MetadataServiceEndpointMode,
		Logger:                         baselog,
	}

	if c.SkipMetadataAPICheck != nil {
		if *c.SkipMetadataAPICheck {
			cfg.EC2MetadataServiceEnableState = imds.ClientDisabled
		} else {
			cfg.EC2MetadataServiceEnableState = imds.ClientEnabled
		}
	}

	// This is probably bugged, but it replicates the s3 backend behavior exactly
	if len(c.SharedCredentialsFile) != 0 {
		cfg.SharedCredentialsFiles = []string{c.SharedCredentialsFile}
	}
	if len(c.SharedCredentialsFiles) != 0 {
		cfg.SharedCredentialsFiles = c.SharedCredentialsFiles
	} else {
		envFile := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
		if len(envFile) != 0 {
			cfg.SharedCredentialsFiles = []string{envFile}
		}
	}

	if len(c.SharedConfigFiles) != 0 {
		cfg.SharedConfigFiles = c.SharedConfigFiles
	} else {
		envFile := os.Getenv("AWS_SHARED_CONFIG_FILE")
		if len(envFile) != 0 {
			cfg.SharedConfigFiles = []string{envFile}
		}
	}

	if c.AssumeRole != nil {
		cfg.AssumeRole = &awsbase.AssumeRole{
			RoleARN:           c.AssumeRole.RoleARN,
			ExternalID:        c.AssumeRole.ExternalID,
			Policy:            strings.TrimSpace(c.AssumeRole.Policy),
			PolicyARNs:        c.AssumeRole.PolicyARNs,
			SessionName:       c.AssumeRole.SessionName,
			Tags:              c.AssumeRole.Tags,
			TransitiveTagKeys: c.AssumeRole.TransitiveTagKeys,
		}

		// Parse duration
		if len(c.AssumeRole.Duration) != 0 {
			dur, err := time.ParseDuration(c.AssumeRole.Duration)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid assume_role duration %q: %w", c.AssumeRole.Duration, err)
			}
			cfg.AssumeRole.Duration = dur
		}
	}
	if c.AssumeRoleWithWebIdentity != nil {
		cfg.AssumeRoleWithWebIdentity = &awsbase.AssumeRoleWithWebIdentity{
			RoleARN:              c.AssumeRoleWithWebIdentity.RoleARN,
			Policy:               strings.TrimSpace(c.AssumeRoleWithWebIdentity.Policy),
			PolicyARNs:           c.AssumeRoleWithWebIdentity.PolicyARNs,
			SessionName:          c.AssumeRoleWithWebIdentity.SessionName,
			WebIdentityToken:     c.AssumeRoleWithWebIdentity.WebIdentityToken,
			WebIdentityTokenFile: c.AssumeRoleWithWebIdentity.WebIdentityTokenFile,
		}

		// Env defaults
		if len(cfg.AssumeRoleWithWebIdentity.RoleARN) == 0 {
			cfg.AssumeRoleWithWebIdentity.RoleARN = os.Getenv("AWS_ROLE_ARN")
		}
		if len(cfg.AssumeRoleWithWebIdentity.SessionName) == 0 {
			cfg.AssumeRoleWithWebIdentity.SessionName = os.Getenv("AWS_ROLE_SESSION_NAME")
		}
		if len(cfg.AssumeRoleWithWebIdentity.WebIdentityToken) == 0 {
			cfg.AssumeRoleWithWebIdentity.WebIdentityToken = os.Getenv("AWS_WEB_IDENTITY_TOKEN")
		}
		if len(cfg.AssumeRoleWithWebIdentity.WebIdentityTokenFile) == 0 {
			cfg.AssumeRoleWithWebIdentity.WebIdentityTokenFile = os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE")
		}

		// Parse duration
		if len(c.AssumeRoleWithWebIdentity.Duration) != 0 {
			dur, err := time.ParseDuration(c.AssumeRoleWithWebIdentity.Duration)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid assume_role_with_web_identity duration %q: %w", c.AssumeRoleWithWebIdentity.Duration, err)
			}
			cfg.AssumeRoleWithWebIdentity.Duration = dur
		}
	}

	cfg.AllowedAccountIds = c.AllowedAccountIds
	cfg.ForbiddenAccountIds = c.ForbiddenAccountIds

	if len(c.RetryMode) != 0 {
		mode, err := aws.ParseRetryMode(c.RetryMode)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid retry mode %q: %w", c.RetryMode, err)
		}
		cfg.RetryMode = mode
	}

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
		svc:    kms.NewFromConfig(awsConfig),
		ctx:    ctx,
	}, new(keyMeta), nil
}

func includeProtoIfNessesary(endpoint string) string {
	if matched, _ := regexp.MatchString("[a-z]*://.*", endpoint); !matched {
		log.Printf("[DEBUG] Adding https:// prefix to endpoint '%s'", endpoint)
		endpoint = fmt.Sprintf("https://%s", endpoint)
	}
	return endpoint
}

func attachLoggerToContext(ctx context.Context) (context.Context, baselogging.HcLogger) {
	ctx, baselog := baselogging.NewHcLogger(ctx, logging.HCLogger().Named("backend-s3"))
	ctx = baselogging.RegisterLogger(ctx, baselog)
	return ctx, baselog
}
