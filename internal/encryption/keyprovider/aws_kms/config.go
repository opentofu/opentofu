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
	"github.com/aws/aws-sdk-go/aws/arn"
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

	// Mirrored S3 Backend Config, mirror any changes
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

// Mirrored from s3 backend config
func includeProtoIfNessesary(endpoint string) string {
	if matched, _ := regexp.MatchString("[a-z]*://.*", endpoint); !matched {
		log.Printf("[DEBUG] Adding https:// prefix to endpoint '%s'", endpoint)
		endpoint = fmt.Sprintf("https://%s", endpoint)
	}
	return endpoint
}

func (c Config) getEndpoints() (ConfigEndpoints, error) {
	endpoints := ConfigEndpoints{}

	// Make sure we have 0 or 1 endpoint blocks
	if len(c.Endpoints) == 1 {
		endpoints = c.Endpoints[1]
	}
	if len(c.Endpoints) > 1 {
		return endpoints, fmt.Errorf("expected single aws_kms endpoints block, multiple provided")
	}

	// Endpoint formatting
	if len(endpoints.IAM) != 0 {
		endpoints.IAM = includeProtoIfNessesary(endpoints.IAM)
	}
	if len(endpoints.STS) != 0 {
		endpoints.STS = includeProtoIfNessesary(endpoints.STS)
	}
	return endpoints, nil
}

func parseAssumeRoleDuration(val string) (dur time.Duration, err error) {
	if len(val) == 0 {
		return dur, nil
	}
	dur, err = time.ParseDuration(val)
	if err != nil {
		return dur, fmt.Errorf("invalid assume_role duration %q: %w", val, err)
	}

	minDur := 15 * time.Minute
	maxDur := 12 * time.Hour
	if (minDur > 0 && dur < minDur) || (maxDur > 0 && dur > maxDur) {
		return dur, fmt.Errorf("assume_role duration must be between %s and %s, had %s", minDur, maxDur, dur)
	}
	return dur, nil
}

func validatePolicyARNs(arns []string) error {
	for _, v := range arns {
		arn, err := arn.Parse(v)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(arn.Resource, "policy/") {
			return fmt.Errorf("arn must be a valid IAM Policy ARN, got %q", v)
		}
	}
	return nil
}

func (c Config) getAssumeRole() (*awsbase.AssumeRole, error) {
	if c.AssumeRole == nil {
		return nil, nil
	}

	duration, err := parseAssumeRoleDuration(c.AssumeRole.Duration)
	if err != nil {
		return nil, err
	}

	err = validatePolicyARNs(c.AssumeRole.PolicyARNs)
	if err != nil {
		return nil, err
	}

	assumeRole := &awsbase.AssumeRole{
		RoleARN:           c.AssumeRole.RoleARN,
		Duration:          duration,
		ExternalID:        c.AssumeRole.ExternalID,
		Policy:            c.AssumeRole.Policy,
		PolicyARNs:        c.AssumeRole.PolicyARNs,
		SessionName:       c.AssumeRole.SessionName,
		Tags:              c.AssumeRole.Tags,
		TransitiveTagKeys: c.AssumeRole.TransitiveTagKeys,
	}
	return assumeRole, nil
}
func (c Config) getAssumeRoleWithWebIdentity() (*awsbase.AssumeRoleWithWebIdentity, error) {
	if c.AssumeRoleWithWebIdentity == nil {
		return nil, nil
	}

	if c.AssumeRoleWithWebIdentity.WebIdentityToken != "" && c.AssumeRoleWithWebIdentity.WebIdentityTokenFile != "" {
		return nil, fmt.Errorf("conflicting config attributes: only web_identity_token or web_identity_token_file can be specified, not both")
	}

	duration, err := parseAssumeRoleDuration(c.AssumeRoleWithWebIdentity.Duration)
	if err != nil {
		return nil, err
	}

	err = validatePolicyARNs(c.AssumeRoleWithWebIdentity.PolicyARNs)
	if err != nil {
		return nil, err
	}

	return &awsbase.AssumeRoleWithWebIdentity{
		RoleARN:              stringAttrEnvFallback(c.AssumeRoleWithWebIdentity.RoleARN, "AWS_ROLE_ARN"),
		Duration:             duration,
		Policy:               c.AssumeRoleWithWebIdentity.Policy,
		PolicyARNs:           c.AssumeRoleWithWebIdentity.PolicyARNs,
		SessionName:          stringAttrEnvFallback(c.AssumeRoleWithWebIdentity.SessionName, "AWS_ROLE_SESSION_NAME"),
		WebIdentityToken:     stringAttrEnvFallback(c.AssumeRoleWithWebIdentity.WebIdentityToken, "AWS_WEB_IDENTITY_TOKEN"),
		WebIdentityTokenFile: stringAttrEnvFallback(c.AssumeRoleWithWebIdentity.WebIdentityTokenFile, "AWS_WEB_IDENTITY_TOKEN_FILE"),
	}, nil
}

func (c Config) ToAWSBaseConfig() (*awsbase.Config, error) {
	// Get endpoints to use
	endpoints, err := c.getEndpoints()
	if err != nil {
		return nil, err
	}

	// Get assume role
	assumeRole, err := c.getAssumeRole()
	if err != nil {
		return nil, err
	}

	// Get assume role with web identity
	assumeRoleWithWebIdentity, err := c.getAssumeRoleWithWebIdentity()
	if err != nil {
		return nil, err
	}

	// Validate region
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

	// Validate account_ids
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
		AssumeRole:                assumeRole,
		AssumeRoleWithWebIdentity: assumeRoleWithWebIdentity,
		AllowedAccountIds:         c.AllowedAccountIds,
		ForbiddenAccountIds:       c.ForbiddenAccountIds,
	}, nil
}

func (c Config) Build() (keyprovider.KeyProvider, keyprovider.KeyMeta, error) {
	cfg, err := c.ToAWSBaseConfig()
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
		svc:    kms.NewFromConfig(awsConfig),
		ctx:    ctx,
	}, new(keyMeta), nil
}

// Mirrored from s3 backend config
func attachLoggerToContext(ctx context.Context) (context.Context, baselogging.HcLogger) {
	ctx, baselog := baselogging.NewHcLogger(ctx, logging.HCLogger().Named("backend-s3"))
	ctx = baselogging.RegisterLogger(ctx, baselog)
	return ctx, baselog
}
