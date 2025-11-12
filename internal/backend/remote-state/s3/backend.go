// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package s3

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	awsbase "github.com/hashicorp/aws-sdk-go-base/v2"
	baselogging "github.com/hashicorp/aws-sdk-go-base/v2/logging"
	awsbaseValidation "github.com/hashicorp/aws-sdk-go-base/v2/validation"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/version"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
)

func New(enc encryption.StateEncryption) backend.Backend {
	return &Backend{encryption: enc}
}

type Backend struct {
	encryption encryption.StateEncryption
	s3Client   *s3.Client
	dynClient  *dynamodb.Client
	awsConfig  aws.Config

	bucketName            string
	keyName               string
	serverSideEncryption  bool
	customerEncryptionKey []byte
	acl                   string
	lockTags              map[string]string
	stateTags             map[string]string
	kmsKeyID              string
	ddbTable              string
	workspaceKeyPrefix    string
	skipS3Checksum        bool
	useLockfile           bool
}

// ConfigSchema returns a description of the expected configuration
// structure for the receiving backend.
// This structure is mirrored by the encryption aws_kms key provider and should be kept in sync.
func (b *Backend) ConfigSchema() *configschema.Block {
	return &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"bucket": {
				Type:        cty.String,
				Required:    true,
				Description: "The name of the S3 bucket",
			},
			"key": {
				Type:        cty.String,
				Required:    true,
				Description: "The path to the state file inside the bucket",
			},
			"region": {
				Type:        cty.String,
				Optional:    true,
				Description: "AWS region of the S3 Bucket and DynamoDB Table (if used).",
			},
			"endpoints": {
				Optional: true,
				NestedType: &configschema.Object{
					Nesting: configschema.NestingSingle,
					Attributes: map[string]*configschema.Attribute{
						"s3": {
							Type:        cty.String,
							Optional:    true,
							Description: "A custom endpoint for the S3 API.",
						},
						"iam": {
							Type:        cty.String,
							Optional:    true,
							Description: "A custom endpoint for the IAM API.",
						},
						"sts": {
							Type:        cty.String,
							Optional:    true,
							Description: "A custom endpoint for the STS API.",
						},
						"dynamodb": {
							Type:        cty.String,
							Optional:    true,
							Description: "A custom endpoint for the DynamoDB API.",
						},
					},
				},
			},
			"dynamodb_endpoint": {
				Type:        cty.String,
				Optional:    true,
				Description: "A custom endpoint for the DynamoDB API. Use `endpoints.dynamodb` instead.",
				Deprecated:  true,
			},
			"endpoint": {
				Type:        cty.String,
				Optional:    true,
				Description: "A custom endpoint for the S3 API. Use `endpoints.s3` instead",
				Deprecated:  true,
			},
			"iam_endpoint": {
				Type:        cty.String,
				Optional:    true,
				Description: "A custom endpoint for the IAM API. Use `endpoints.iam` instead",
				Deprecated:  true,
			},
			"sts_endpoint": {
				Type:        cty.String,
				Optional:    true,
				Description: "A custom endpoint for the STS API. Use `endpoints.sts` instead",
				Deprecated:  true,
			},
			"sts_region": {
				Type:        cty.String,
				Optional:    true,
				Description: "The region where AWS STS operations will take place",
			},
			"encrypt": {
				Type:        cty.Bool,
				Optional:    true,
				Description: "Whether to enable server side encryption of the state file",
			},
			"acl": {
				Type:        cty.String,
				Optional:    true,
				Description: "Canned ACL to be applied to the state file",
			},
			"state_tags": {
				Type:        cty.Map(cty.String),
				Optional:    true,
				Description: "Tags to be applied to the state object",
			},
			"lock_tags": {
				Type:        cty.Map(cty.String),
				Optional:    true,
				Description: "Tags to be applied to the lock object",
			},
			"access_key": {
				Type:        cty.String,
				Optional:    true,
				Description: "AWS access key",
			},
			"secret_key": {
				Type:        cty.String,
				Optional:    true,
				Description: "AWS secret key",
			},
			"kms_key_id": {
				Type:        cty.String,
				Optional:    true,
				Description: "The ARN of a KMS Key to use for encrypting the state",
			},
			"dynamodb_table": {
				Type:        cty.String,
				Optional:    true,
				Description: "DynamoDB table for state locking and consistency",
			},
			"profile": {
				Type:        cty.String,
				Optional:    true,
				Description: "AWS profile name",
			},
			"shared_credentials_file": {
				Type:        cty.String,
				Optional:    true,
				Description: "Path to a shared credentials file",
			},
			"shared_credentials_files": {
				Type:        cty.Set(cty.String),
				Optional:    true,
				Description: "Paths to a shared credentials files",
			},
			"shared_config_files": {
				Type:        cty.Set(cty.String),
				Optional:    true,
				Description: "Paths to shared config files",
			},
			"token": {
				Type:        cty.String,
				Optional:    true,
				Description: "MFA token",
			},
			"skip_credentials_validation": {
				Type:        cty.Bool,
				Optional:    true,
				Description: "Skip the credentials validation via STS API.",
			},
			"skip_metadata_api_check": {
				Type:        cty.Bool,
				Optional:    true,
				Description: "Skip the AWS Metadata API check.",
			},
			"skip_region_validation": {
				Type:        cty.Bool,
				Optional:    true,
				Description: "Skip static validation of region name.",
			},
			"skip_requesting_account_id": {
				Type:        cty.Bool,
				Optional:    true,
				Description: "Skip requesting the account ID. Useful for AWS API implementations that do not have the IAM, STS API, or metadata API.",
			},
			"sse_customer_key": {
				Type:        cty.String,
				Optional:    true,
				Description: "The base64-encoded encryption key to use for server-side encryption with customer-provided keys (SSE-C).",
				Sensitive:   true,
			},
			"role_arn": {
				Type:        cty.String,
				Optional:    true,
				Description: "The role to be assumed",
				Deprecated:  true,
			},
			"session_name": {
				Type:        cty.String,
				Optional:    true,
				Description: "The session name to use when assuming the role.",
				Deprecated:  true,
			},
			"external_id": {
				Type:        cty.String,
				Optional:    true,
				Description: "The external ID to use when assuming the role",
				Deprecated:  true,
			},
			"assume_role_duration_seconds": {
				Type:        cty.Number,
				Optional:    true,
				Description: "Seconds to restrict the assume role session duration.",
				Deprecated:  true,
			},
			"assume_role_policy": {
				Type:        cty.String,
				Optional:    true,
				Description: "IAM Policy JSON describing further restricting permissions for the IAM Role being assumed.",
				Deprecated:  true,
			},
			"assume_role_policy_arns": {
				Type:        cty.Set(cty.String),
				Optional:    true,
				Description: "Amazon Resource Names (ARNs) of IAM Policies describing further restricting permissions for the IAM Role being assumed.",
				Deprecated:  true,
			},
			"assume_role_tags": {
				Type:        cty.Map(cty.String),
				Optional:    true,
				Description: "Assume role session tags.",
				Deprecated:  true,
			},
			"assume_role_transitive_tag_keys": {
				Type:        cty.Set(cty.String),
				Optional:    true,
				Description: "Assume role session tag keys to pass to any subsequent sessions.",
				Deprecated:  true,
			},
			"workspace_key_prefix": {
				Type:        cty.String,
				Optional:    true,
				Description: "The prefix applied to the non-default state path inside the bucket.",
			},
			"force_path_style": {
				Type:        cty.Bool,
				Optional:    true,
				Description: "Force s3 to use path style api. Use `use_path_style` instead.",
				Deprecated:  true,
			},
			"use_path_style": {
				Type:        cty.Bool,
				Optional:    true,
				Description: "Enable path-style S3 URLs.",
			},
			"retry_mode": {
				Type:        cty.String,
				Optional:    true,
				Description: "Specifies how retries are attempted. Valid values are `standard` and `adaptive`.",
			},
			"max_retries": {
				Type:        cty.Number,
				Optional:    true,
				Description: "The maximum number of times an AWS API request is retried on retryable failure.",
			},
			"custom_ca_bundle": {
				Type:        cty.String,
				Optional:    true,
				Description: "File containing custom root and intermediate certificates. Can also be configured using the `AWS_CA_BUNDLE` environment variable.",
			},
			"ec2_metadata_service_endpoint": {
				Type:        cty.String,
				Optional:    true,
				Description: "The endpoint of IMDS.",
			},
			"ec2_metadata_service_endpoint_mode": {
				Type:        cty.String,
				Optional:    true,
				Description: "The endpoint mode of IMDS. Valid values: IPv4, IPv6.",
			},
			"assume_role": {
				Optional: true,
				NestedType: &configschema.Object{
					Nesting: configschema.NestingSingle,
					Attributes: map[string]*configschema.Attribute{
						"role_arn": {
							Type:        cty.String,
							Required:    true,
							Description: "The role to be assumed.",
						},
						"duration": {
							Type:        cty.String,
							Optional:    true,
							Description: "Seconds to restrict the assume role session duration.",
						},
						"external_id": {
							Type:        cty.String,
							Optional:    true,
							Description: "The external ID to use when assuming the role",
						},
						"policy": {
							Type:        cty.String,
							Optional:    true,
							Description: "IAM Policy JSON describing further restricting permissions for the IAM Role being assumed.",
						},
						"policy_arns": {
							Type:        cty.Set(cty.String),
							Optional:    true,
							Description: "Amazon Resource Names (ARNs) of IAM Policies describing further restricting permissions for the IAM Role being assumed.",
						},
						"session_name": {
							Type:        cty.String,
							Optional:    true,
							Description: "The session name to use when assuming the role.",
						},
						"tags": {
							Type:        cty.Map(cty.String),
							Optional:    true,
							Description: "Assume role session tags.",
						},
						"transitive_tag_keys": {
							Type:        cty.Set(cty.String),
							Optional:    true,
							Description: "Assume role session tag keys to pass to any subsequent sessions.",
						},
						//
						// NOT SUPPORTED by `aws-sdk-go-base/v1`
						// Cannot be added yet.
						//
						// "source_identity": stringAttribute{
						// 	configschema.Attribute{
						// 		Type:         cty.String,
						// 		Optional:     true,
						// 		Description:  "Source identity specified by the principal assuming the role.",
						// 		ValidateFunc: validAssumeRoleSourceIdentity,
						// 	},
						// },
					},
				},
			},
			"assume_role_with_web_identity": {
				Optional: true,
				NestedType: &configschema.Object{
					Nesting: configschema.NestingSingle,
					Attributes: map[string]*configschema.Attribute{
						"role_arn": {
							Type:        cty.String,
							Optional:    true,
							Description: "The Amazon Resource Name (ARN) role to assume.",
						},
						"web_identity_token": {
							Type:        cty.String,
							Optional:    true,
							Sensitive:   true,
							Description: "The OAuth 2.0 access token or OpenID Connect ID token that is provided by the identity provider.",
						},
						"web_identity_token_file": {
							Type:        cty.String,
							Optional:    true,
							Description: "The path to a file which contains an OAuth 2.0 access token or OpenID Connect ID token that is provided by the identity provider.",
						},
						"session_name": {
							Type:        cty.String,
							Optional:    true,
							Description: "The name applied to this assume-role session.",
						},
						"policy": {
							Type:        cty.String,
							Optional:    true,
							Description: "IAM Policy JSON describing further restricting permissions for the IAM Role being assumed.",
						},
						"policy_arns": {
							Type:        cty.Set(cty.String),
							Optional:    true,
							Description: "Amazon Resource Names (ARNs) of IAM Policies describing further restricting permissions for the IAM Role being assumed.",
						},
						"duration": {
							Type:        cty.String,
							Optional:    true,
							Description: "The duration, between 15 minutes and 12 hours, of the role session. Valid time units are ns, us (or µs), ms, s, h, or m.",
						},
					},
				},
			},
			"forbidden_account_ids": {
				Type:        cty.Set(cty.String),
				Optional:    true,
				Description: "List of forbidden AWS account IDs.",
			},
			"allowed_account_ids": {
				Type:        cty.Set(cty.String),
				Optional:    true,
				Description: "List of allowed AWS account IDs.",
			},
			"http_proxy": {
				Type:        cty.String,
				Optional:    true,
				Description: "The address of an HTTP proxy to use when accessing the AWS API.",
			},
			"https_proxy": {
				Type:        cty.String,
				Optional:    true,
				Description: "The address of an HTTPS proxy to use when accessing the AWS API.",
			},
			"no_proxy": {
				Type:     cty.String,
				Optional: true,
				Description: `Comma-separated values which specify hosts that should be excluded from proxying.
See details: https://cs.opensource.google/go/x/net/+/refs/tags/v0.17.0:http/httpproxy/proxy.go;l=38-50.`,
			},
			"insecure": {
				Type:        cty.Bool,
				Optional:    true,
				Description: "Explicitly allow the backend to perform \"insecure\" SSL requests.",
			},
			"use_dualstack_endpoint": {
				Type:        cty.Bool,
				Optional:    true,
				Description: "Resolve an endpoint with DualStack capability.",
			},
			"use_fips_endpoint": {
				Type:        cty.Bool,
				Optional:    true,
				Description: "Resolve an endpoint with FIPS capability.",
			},
			"skip_s3_checksum": {
				Type:        cty.Bool,
				Optional:    true,
				Description: "Do not include checksum when uploading S3 Objects. Useful for some S3-Compatible APIs as some of them do not support checksum checks.",
			},
			"use_lockfile": {
				Type:        cty.Bool,
				Optional:    true,
				Description: "Manage locking in the same configured S3 bucket",
			},
		},
	}
}

// PrepareConfig checks the validity of the values in the given
// configuration, and inserts any missing defaults, assuming that its
// structure has already been validated per the schema returned by
// ConfigSchema.
func (b *Backend) PrepareConfig(obj cty.Value) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	if obj.IsNull() {
		return obj, diags
	}

	if val := obj.GetAttr("bucket"); val.IsNull() || val.AsString() == "" {
		diags = diags.Append(tfdiags.AttributeValue(
			tfdiags.Error,
			"Invalid bucket value",
			`The "bucket" attribute value must not be empty.`,
			cty.Path{cty.GetAttrStep{Name: "bucket"}},
		))
	}

	if val := obj.GetAttr("key"); val.IsNull() || val.AsString() == "" {
		diags = diags.Append(tfdiags.AttributeValue(
			tfdiags.Error,
			"Invalid key value",
			`The "key" attribute value must not be empty.`,
			cty.Path{cty.GetAttrStep{Name: "key"}},
		))
	} else if strings.HasPrefix(val.AsString(), "/") || strings.HasSuffix(val.AsString(), "/") {
		// S3 will strip leading slashes from an object, so while this will
		// technically be accepted by S3, it will break our workspace hierarchy.
		// S3 will recognize objects with a trailing slash as a directory
		// so they should not be valid keys
		diags = diags.Append(tfdiags.AttributeValue(
			tfdiags.Error,
			"Invalid key value",
			`The "key" attribute value must not start or end with with "/".`,
			cty.Path{cty.GetAttrStep{Name: "key"}},
		))
	}

	if val := obj.GetAttr("region"); val.IsNull() || val.AsString() == "" {
		if os.Getenv("AWS_REGION") == "" && os.Getenv("AWS_DEFAULT_REGION") == "" {
			diags = diags.Append(tfdiags.AttributeValue(
				tfdiags.Error,
				"Missing region value",
				`The "region" attribute or the "AWS_REGION" or "AWS_DEFAULT_REGION" environment variables must be set.`,
				cty.Path{cty.GetAttrStep{Name: "region"}},
			))
		}
	}

	if val := obj.GetAttr("kms_key_id"); !val.IsNull() && val.AsString() != "" {
		if val := obj.GetAttr("sse_customer_key"); !val.IsNull() && val.AsString() != "" {
			diags = diags.Append(tfdiags.AttributeValue(
				tfdiags.Error,
				"Invalid encryption configuration",
				encryptionKeyConflictError,
				cty.Path{},
			))
		} else if customerKey := os.Getenv("AWS_SSE_CUSTOMER_KEY"); customerKey != "" {
			diags = diags.Append(tfdiags.AttributeValue(
				tfdiags.Error,
				"Invalid encryption configuration",
				encryptionKeyConflictEnvVarError,
				cty.Path{},
			))
		}

		diags = diags.Append(validateKMSKey(cty.Path{cty.GetAttrStep{Name: "kms_key_id"}}, val.AsString()))
	}

	if val := obj.GetAttr("workspace_key_prefix"); !val.IsNull() {
		if v := val.AsString(); strings.HasPrefix(v, "/") || strings.HasSuffix(v, "/") {
			diags = diags.Append(tfdiags.AttributeValue(
				tfdiags.Error,
				"Invalid workspace_key_prefix value",
				`The "workspace_key_prefix" attribute value must not start with "/".`,
				cty.Path{cty.GetAttrStep{Name: "workspace_key_prefix"}},
			))
		}
	}

	validateAttributesConflict(
		cty.GetAttrPath("shared_credentials_file"),
		cty.GetAttrPath("shared_credentials_files"),
	)(obj, cty.Path{}, &diags)

	attrPath := cty.GetAttrPath("shared_credentials_file")
	if val := obj.GetAttr("shared_credentials_file"); !val.IsNull() {
		detail := fmt.Sprintf(
			`Parameter "%s" is deprecated. Use "%s" instead.`,
			pathString(attrPath),
			pathString(cty.GetAttrPath("shared_credentials_files")))

		diags = diags.Append(attributeWarningDiag(
			"Deprecated Parameter",
			detail,
			attrPath))
	}

	if val := obj.GetAttr("force_path_style"); !val.IsNull() {
		attrPath := cty.GetAttrPath("force_path_style")
		detail := fmt.Sprintf(
			`Parameter "%s" is deprecated. Use "%s" instead.`,
			pathString(attrPath),
			pathString(cty.GetAttrPath("use_path_style")))

		diags = diags.Append(attributeWarningDiag(
			"Deprecated Parameter",
			detail,
			attrPath))
	}

	validateAttributesConflict(
		cty.GetAttrPath("force_path_style"),
		cty.GetAttrPath("use_path_style"),
	)(obj, cty.Path{}, &diags)

	var assumeRoleDeprecatedFields = map[string]string{
		"role_arn":                        "assume_role.role_arn",
		"session_name":                    "assume_role.session_name",
		"external_id":                     "assume_role.external_id",
		"assume_role_duration_seconds":    "assume_role.duration",
		"assume_role_policy":              "assume_role.policy",
		"assume_role_policy_arns":         "assume_role.policy_arns",
		"assume_role_tags":                "assume_role.tags",
		"assume_role_transitive_tag_keys": "assume_role.transitive_tag_keys",
	}

	if val := obj.GetAttr("assume_role"); !val.IsNull() {
		diags = diags.Append(validateNestedAssumeRole(val, cty.Path{cty.GetAttrStep{Name: "assume_role"}}))

		if defined := findDeprecatedFields(obj, assumeRoleDeprecatedFields); len(defined) != 0 {
			diags = diags.Append(tfdiags.WholeContainingBody(
				tfdiags.Error,
				"Conflicting Parameters",
				`The following deprecated parameters conflict with the parameter "assume_role". Replace them as follows:`+"\n"+
					formatDeprecated(defined),
			))
		}
	} else {
		if defined := findDeprecatedFields(obj, assumeRoleDeprecatedFields); len(defined) != 0 {
			diags = diags.Append(tfdiags.WholeContainingBody(
				tfdiags.Warning,
				"Deprecated Parameters",
				`The following parameters have been deprecated. Replace them as follows:`+"\n"+
					formatDeprecated(defined),
			))
		}
	}

	if val := obj.GetAttr("assume_role_with_web_identity"); !val.IsNull() {
		diags = diags.Append(validateAssumeRoleWithWebIdentity(val, cty.GetAttrPath("assume_role_with_web_identity")))
	}

	validateAttributesConflict(
		cty.GetAttrPath("allowed_account_ids"),
		cty.GetAttrPath("forbidden_account_ids"),
	)(obj, cty.Path{}, &diags)

	if val := obj.GetAttr("retry_mode"); !val.IsNull() {
		s := val.AsString()
		if _, err := aws.ParseRetryMode(s); err != nil {
			diags = diags.Append(tfdiags.AttributeValue(
				tfdiags.Error,
				"Invalid retry mode",
				fmt.Sprintf("Valid values are %q and %q.", aws.RetryModeStandard, aws.RetryModeAdaptive),
				cty.Path{cty.GetAttrStep{Name: "retry_mode"}},
			))
		}
	}

	for _, endpoint := range customEndpoints {
		endpoint.Validate(obj, &diags)
	}

	return obj, diags
}

// Configure uses the provided configuration to set configuration fields
// within the backend.
//
// The given configuration is assumed to have already been validated
// against the schema returned by ConfigSchema and passed validation
// via PrepareConfig.
func (b *Backend) Configure(ctx context.Context, obj cty.Value) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if obj.IsNull() {
		return diags
	}

	var region string
	if v, ok := stringAttrOk(obj, "region"); ok {
		region = v
	}

	if region != "" && !boolAttr(obj, "skip_region_validation") {
		if err := awsbaseValidation.SupportedRegion(region); err != nil {
			diags = diags.Append(tfdiags.AttributeValue(
				tfdiags.Error,
				"Invalid region value",
				err.Error(),
				cty.Path{cty.GetAttrStep{Name: "region"}},
			))
			return diags
		}
	}

	b.bucketName = stringAttr(obj, "bucket")
	b.keyName = stringAttr(obj, "key")
	b.acl = stringAttr(obj, "acl")
	if val, ok := stringMapAttrOk(obj, "state_tags"); ok {
		b.stateTags = val
	}
	if val, ok := stringMapAttrOk(obj, "lock_tags"); ok {
		b.lockTags = val
	}
	b.workspaceKeyPrefix = stringAttrDefault(obj, "workspace_key_prefix", "env:")
	b.serverSideEncryption = boolAttr(obj, "encrypt")
	b.kmsKeyID = stringAttr(obj, "kms_key_id")
	b.ddbTable = stringAttr(obj, "dynamodb_table")
	b.useLockfile = boolAttr(obj, "use_lockfile")
	b.skipS3Checksum = boolAttr(obj, "skip_s3_checksum")

	if customerKey, ok := stringAttrOk(obj, "sse_customer_key"); ok {
		if len(customerKey) != 44 {
			diags = diags.Append(tfdiags.AttributeValue(
				tfdiags.Error,
				"Invalid sse_customer_key value",
				"sse_customer_key must be 44 characters in length",
				cty.Path{cty.GetAttrStep{Name: "sse_customer_key"}},
			))
		} else {
			var err error
			if b.customerEncryptionKey, err = base64.StdEncoding.DecodeString(customerKey); err != nil {
				diags = diags.Append(tfdiags.AttributeValue(
					tfdiags.Error,
					"Invalid sse_customer_key value",
					fmt.Sprintf("sse_customer_key must be base64 encoded: %s", err),
					cty.Path{cty.GetAttrStep{Name: "sse_customer_key"}},
				))
			}
		}
	} else if customerKey := os.Getenv("AWS_SSE_CUSTOMER_KEY"); customerKey != "" {
		if len(customerKey) != 44 {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid AWS_SSE_CUSTOMER_KEY value",
				`The environment variable "AWS_SSE_CUSTOMER_KEY" must be 44 characters in length`,
			))
		} else {
			var err error
			if b.customerEncryptionKey, err = base64.StdEncoding.DecodeString(customerKey); err != nil {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Invalid AWS_SSE_CUSTOMER_KEY value",
					fmt.Sprintf(`The environment variable "AWS_SSE_CUSTOMER_KEY" must be base64 encoded: %s`, err),
				))
			}
		}
	}

	ctx, baselog := attachLoggerToContext(ctx)

	cfg := &awsbase.Config{
		AccessKey:               stringAttr(obj, "access_key"),
		CallerDocumentationURL:  "https://opentofu.org/docs/language/settings/backends/s3",
		CallerName:              "S3 Backend",
		IamEndpoint:             customEndpoints["iam"].String(obj),
		MaxRetries:              intAttrDefault(obj, "max_retries", 5),
		Profile:                 stringAttr(obj, "profile"),
		Region:                  stringAttr(obj, "region"),
		SecretKey:               stringAttr(obj, "secret_key"),
		SkipCredsValidation:     boolAttr(obj, "skip_credentials_validation"),
		SkipRequestingAccountId: boolAttr(obj, "skip_requesting_account_id"),
		StsEndpoint:             customEndpoints["sts"].String(obj),
		StsRegion:               stringAttr(obj, "sts_region"),
		Token:                   stringAttr(obj, "token"),

		// Note: we don't need to read env variables explicitly because they are read implicitly by aws-sdk-base-go:
		// see: https://github.com/hashicorp/aws-sdk-go-base/blob/v2.0.0-beta.41/internal/config/config.go#L133
		// which relies on: https://cs.opensource.google/go/x/net/+/refs/tags/v0.18.0:http/httpproxy/proxy.go;l=89-96
		//
		// Note: we are switching to "separate" mode here since the legacy mode is deprecated and should no longer be
		// used.
		HTTPProxyMode:        awsbase.HTTPProxyModeSeparate,
		Insecure:             boolAttr(obj, "insecure"),
		UseDualStackEndpoint: boolAttr(obj, "use_dualstack_endpoint"),
		UseFIPSEndpoint:      boolAttr(obj, "use_fips_endpoint"),
		APNInfo: &awsbase.APNInfo{
			PartnerName: "OpenTofu-S3-Backend",
			Products: []awsbase.UserAgentProduct{
				{Name: httpclient.DefaultApplicationName, Version: version.String()},
			},
		},
		CustomCABundle:                 stringAttrDefaultEnvVar(obj, "custom_ca_bundle", "AWS_CA_BUNDLE"),
		EC2MetadataServiceEndpoint:     stringAttrDefaultEnvVar(obj, "ec2_metadata_service_endpoint", "AWS_EC2_METADATA_SERVICE_ENDPOINT"),
		EC2MetadataServiceEndpointMode: stringAttrDefaultEnvVar(obj, "ec2_metadata_service_endpoint_mode", "AWS_EC2_METADATA_SERVICE_ENDPOINT_MODE"),
		Logger:                         baselog,
	}

	if val, ok := stringAttrOk(obj, "http_proxy"); ok {
		cfg.HTTPProxy = &val
	}
	if val, ok := stringAttrOk(obj, "https_proxy"); ok {
		cfg.HTTPSProxy = &val
	}
	if val, ok := stringAttrOk(obj, "no_proxy"); ok {
		cfg.NoProxy = val
	}

	if val, ok := boolAttrOk(obj, "skip_metadata_api_check"); ok {
		if val {
			cfg.EC2MetadataServiceEnableState = imds.ClientDisabled
		} else {
			cfg.EC2MetadataServiceEnableState = imds.ClientEnabled
		}
	}

	if val, ok := stringAttrOk(obj, "shared_credentials_file"); ok {
		cfg.SharedCredentialsFiles = []string{val}
	}

	if value := obj.GetAttr("assume_role"); !value.IsNull() {
		cfg.AssumeRole = []awsbase.AssumeRole{configureNestedAssumeRole(obj)}
	} else if value := obj.GetAttr("role_arn"); !value.IsNull() {
		cfg.AssumeRole = []awsbase.AssumeRole{configureAssumeRole(obj)}
	}

	if val := obj.GetAttr("assume_role_with_web_identity"); !val.IsNull() {
		cfg.AssumeRoleWithWebIdentity = configureAssumeRoleWithWebIdentity(val)
	}

	if val, ok := stringSliceAttrDefaultEnvVarOk(obj, "shared_credentials_files", "AWS_SHARED_CREDENTIALS_FILE"); ok {
		cfg.SharedCredentialsFiles = val
	}
	if val, ok := stringSliceAttrDefaultEnvVarOk(obj, "shared_config_files", "AWS_SHARED_CONFIG_FILE"); ok {
		cfg.SharedConfigFiles = val
	}

	if val, ok := stringSliceAttrOk(obj, "allowed_account_ids"); ok {
		cfg.AllowedAccountIds = val
	}

	if val, ok := stringSliceAttrOk(obj, "forbidden_account_ids"); ok {
		cfg.ForbiddenAccountIds = val
	}

	if val, ok := stringAttrOk(obj, "retry_mode"); ok {
		mode, err := aws.ParseRetryMode(val)
		if err != nil {
			panic(fmt.Sprintf("invalid retry mode %q: %s", val, err))
		}
		cfg.RetryMode = mode
	}

	_, awsConfig, awsDiags := awsbase.GetAwsConfig(ctx, cfg)

	for _, d := range awsDiags {
		diags = diags.Append(tfdiags.Sourceless(
			baseSeverityToTofuSeverity(d.Severity()),
			d.Summary(),
			d.Detail(),
		))
	}

	if d := verifyAllowedAccountID(ctx, awsConfig, cfg); len(d) != 0 {
		diags = diags.Append(d)
	}

	if diags.HasErrors() {
		return diags
	}

	b.awsConfig = awsConfig

	b.dynClient = dynamodb.NewFromConfig(awsConfig, getDynamoDBConfig(obj))

	b.s3Client = s3.NewFromConfig(awsConfig, getS3Config(obj))

	return diags
}

func attachLoggerToContext(ctx context.Context) (context.Context, baselogging.HcLogger) {
	ctx, baselog := baselogging.NewHcLogger(ctx, logging.HCLogger().Named("backend-s3"))
	ctx = baselogging.RegisterLogger(ctx, baselog)
	return ctx, baselog
}

func verifyAllowedAccountID(ctx context.Context, awsConfig aws.Config, cfg *awsbase.Config) tfdiags.Diagnostics {
	if len(cfg.ForbiddenAccountIds) == 0 && len(cfg.AllowedAccountIds) == 0 {
		return nil
	}

	var diags tfdiags.Diagnostics
	accountID, _, awsDiags := awsbase.GetAwsAccountIDAndPartition(ctx, awsConfig, cfg)
	for _, d := range awsDiags {
		diags = diags.Append(tfdiags.Sourceless(
			baseSeverityToTofuSeverity(d.Severity()),
			fmt.Sprintf("Retrieving AWS account details: %s", d.Summary()),
			d.Detail(),
		))
	}

	err := cfg.VerifyAccountIDAllowed(accountID)
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid account ID",
			err.Error(),
		))
	}
	return diags
}

func getDynamoDBConfig(obj cty.Value) func(options *dynamodb.Options) {
	return func(options *dynamodb.Options) {
		if v, ok := customEndpoints["dynamodb"].StringOk(obj); ok {
			options.BaseEndpoint = aws.String(v)
		}
	}
}

func getS3Config(obj cty.Value) func(options *s3.Options) {
	return func(options *s3.Options) {
		if v, ok := customEndpoints["s3"].StringOk(obj); ok {
			options.BaseEndpoint = aws.String(v)
		}
		if v, ok := boolAttrOk(obj, "force_path_style"); ok {
			options.UsePathStyle = v
		}
		if v, ok := boolAttrOk(obj, "use_path_style"); ok {
			options.UsePathStyle = v
		}
	}
}

func configureNestedAssumeRole(obj cty.Value) awsbase.AssumeRole {
	assumeRole := awsbase.AssumeRole{}

	obj = obj.GetAttr("assume_role")
	if val, ok := stringAttrOk(obj, "role_arn"); ok {
		assumeRole.RoleARN = val
	}
	if val, ok := stringAttrOk(obj, "duration"); ok {
		dur, err := time.ParseDuration(val)
		if err != nil {
			// This should never happen because the schema should have
			// already validated the duration.
			panic(fmt.Sprintf("invalid duration %q: %s", val, err))
		}

		assumeRole.Duration = dur
	}
	if val, ok := stringAttrOk(obj, "external_id"); ok {
		assumeRole.ExternalID = val
	}

	if val, ok := stringAttrOk(obj, "policy"); ok {
		assumeRole.Policy = strings.TrimSpace(val)
	}
	if val, ok := stringSliceAttrOk(obj, "policy_arns"); ok {
		assumeRole.PolicyARNs = val
	}
	if val, ok := stringAttrOk(obj, "session_name"); ok {
		assumeRole.SessionName = val
	}
	if val, ok := stringMapAttrOk(obj, "tags"); ok {
		assumeRole.Tags = val
	}
	if val, ok := stringSliceAttrOk(obj, "transitive_tag_keys"); ok {
		assumeRole.TransitiveTagKeys = val
	}

	return assumeRole
}

func configureAssumeRole(obj cty.Value) awsbase.AssumeRole {
	assumeRole := awsbase.AssumeRole{}

	assumeRole.RoleARN = stringAttr(obj, "role_arn")
	assumeRole.Duration = time.Duration(int64(intAttr(obj, "assume_role_duration_seconds")) * int64(time.Second))
	assumeRole.ExternalID = stringAttr(obj, "external_id")
	assumeRole.Policy = stringAttr(obj, "assume_role_policy")
	assumeRole.SessionName = stringAttr(obj, "session_name")

	if val, ok := stringSliceAttrOk(obj, "assume_role_policy_arns"); ok {
		assumeRole.PolicyARNs = val
	}
	if val, ok := stringMapAttrOk(obj, "assume_role_tags"); ok {
		assumeRole.Tags = val
	}
	if val, ok := stringSliceAttrOk(obj, "assume_role_transitive_tag_keys"); ok {
		assumeRole.TransitiveTagKeys = val
	}

	return assumeRole
}

func configureAssumeRoleWithWebIdentity(obj cty.Value) *awsbase.AssumeRoleWithWebIdentity {
	cfg := &awsbase.AssumeRoleWithWebIdentity{
		RoleARN:              stringAttrDefaultEnvVar(obj, "role_arn", "AWS_ROLE_ARN"),
		Policy:               stringAttr(obj, "policy"),
		PolicyARNs:           stringSliceAttr(obj, "policy_arns"),
		SessionName:          stringAttrDefaultEnvVar(obj, "session_name", "AWS_ROLE_SESSION_NAME"),
		WebIdentityToken:     stringAttrDefaultEnvVar(obj, "web_identity_token", "AWS_WEB_IDENTITY_TOKEN"),
		WebIdentityTokenFile: stringAttrDefaultEnvVar(obj, "web_identity_token_file", "AWS_WEB_IDENTITY_TOKEN_FILE"),
	}
	if val, ok := stringAttrOk(obj, "duration"); ok {
		d, err := time.ParseDuration(val)
		if err != nil {
			// This should never happen because the schema should have
			// already validated the duration.
			panic(fmt.Sprintf("invalid duration %q: %s", val, err))
		}
		cfg.Duration = d
	}
	return cfg
}

func stringValue(val cty.Value) string {
	v, _ := stringValueOk(val)
	return v
}

func stringValueOk(val cty.Value) (string, bool) {
	if val.IsNull() {
		return "", false
	} else {
		return val.AsString(), true
	}
}

func stringAttr(obj cty.Value, name string) string {
	return stringValue(obj.GetAttr(name))
}

func stringAttrOk(obj cty.Value, name string) (string, bool) {
	return stringValueOk(obj.GetAttr(name))
}

func stringAttrDefault(obj cty.Value, name, def string) string {
	if v, ok := stringAttrOk(obj, name); !ok {
		return def
	} else {
		return v
	}
}

func stringSliceValue(val cty.Value) []string {
	v, _ := stringSliceValueOk(val)
	return v
}

func stringSliceValueOk(val cty.Value) ([]string, bool) {
	if val.IsNull() {
		return nil, false
	}

	var v []string
	if err := gocty.FromCtyValue(val, &v); err != nil {
		return nil, false
	}
	return v, true
}

func stringSliceAttr(obj cty.Value, name string) []string {
	return stringSliceValue(obj.GetAttr(name))
}

func stringSliceAttrOk(obj cty.Value, name string) ([]string, bool) {
	return stringSliceValueOk(obj.GetAttr(name))
}

func stringSliceAttrDefaultEnvVarOk(obj cty.Value, name string, envvars ...string) ([]string, bool) {
	if v, ok := stringSliceAttrOk(obj, name); !ok {
		for _, envvar := range envvars {
			if ev := os.Getenv(envvar); ev != "" {
				return []string{ev}, true
			}
		}
		return nil, false
	} else {
		return v, true
	}
}

func stringAttrDefaultEnvVar(obj cty.Value, name string, envvars ...string) string {
	if v, ok := stringAttrDefaultEnvVarOk(obj, name, envvars...); !ok {
		return ""
	} else {
		return v
	}
}

func stringAttrDefaultEnvVarOk(obj cty.Value, name string, envvars ...string) (string, bool) {
	if v, ok := stringAttrOk(obj, name); !ok {
		for _, envvar := range envvars {
			if v := os.Getenv(envvar); v != "" {
				return v, true
			}
		}
		return "", false
	} else {
		return v, true
	}
}

func boolAttr(obj cty.Value, name string) bool {
	v, _ := boolAttrOk(obj, name)
	return v
}

func boolAttrOk(obj cty.Value, name string) (bool, bool) {
	if val := obj.GetAttr(name); val.IsNull() {
		return false, false
	} else {
		return val.True(), true
	}
}

func intAttr(obj cty.Value, name string) int {
	v, _ := intAttrOk(obj, name)
	return v
}

func intAttrOk(obj cty.Value, name string) (int, bool) {
	if val := obj.GetAttr(name); val.IsNull() {
		return 0, false
	} else {
		var v int
		if err := gocty.FromCtyValue(val, &v); err != nil {
			return 0, false
		}
		return v, true
	}
}

func intAttrDefault(obj cty.Value, name string, def int) int {
	if v, ok := intAttrOk(obj, name); !ok {
		return def
	} else {
		return v
	}
}

func stringMapValueOk(val cty.Value) (map[string]string, bool) {
	var m map[string]string
	err := gocty.FromCtyValue(val, &m)
	if err != nil {
		return nil, false
	}
	return m, true
}

func stringMapAttrOk(obj cty.Value, name string) (map[string]string, bool) {
	return stringMapValueOk(obj.GetAttr(name))
}

func pathString(path cty.Path) string {
	var buf strings.Builder
	for i, step := range path {
		switch x := step.(type) {
		case cty.GetAttrStep:
			if i != 0 {
				buf.WriteString(".")
			}
			buf.WriteString(x.Name)
		case cty.IndexStep:
			val := x.Key
			typ := val.Type()
			var s string
			switch typ {
			case cty.String:
				s = val.AsString()
			case cty.Number:
				num := val.AsBigFloat()
				if num.IsInt() {
					s = num.Text('f', -1)
				} else {
					s = num.String()
				}
			default:
				s = fmt.Sprintf("<unexpected index: %s>", typ.FriendlyName())
			}
			buf.WriteString(fmt.Sprintf("[%s]", s))
		default:
			if i != 0 {
				buf.WriteString(".")
			}
			buf.WriteString(fmt.Sprintf("<unexpected step: %[1]T %[1]v>", x))
		}
	}
	return buf.String()
}

func findDeprecatedFields(obj cty.Value, attrs map[string]string) map[string]string {
	defined := make(map[string]string)
	for attr, v := range attrs {
		if val := obj.GetAttr(attr); !val.IsNull() {
			defined[attr] = v
		}
	}
	return defined
}

func formatDeprecated(attrs map[string]string) string {
	var maxLen int
	var buf strings.Builder

	names := make([]string, 0, len(attrs))
	for deprecated, replacement := range attrs {
		names = append(names, deprecated)
		if l := len(deprecated); l > maxLen {
			maxLen = l
		}

		fmt.Fprintf(&buf, "  * %-[1]*[2]s -> %s\n", maxLen, deprecated, replacement)
	}

	sort.Strings(names)

	return buf.String()
}

const encryptionKeyConflictError = `Only one of "kms_key_id" and "sse_customer_key" can be set.

The "kms_key_id" is used for encryption with KMS-Managed Keys (SSE-KMS)
while "sse_customer_key" is used for encryption with customer-managed keys (SSE-C).
Please choose one or the other.`

const encryptionKeyConflictEnvVarError = `Only one of "kms_key_id" and the environment variable "AWS_SSE_CUSTOMER_KEY" can be set.

The "kms_key_id" is used for encryption with KMS-Managed Keys (SSE-KMS)
while "AWS_SSE_CUSTOMER_KEY" is used for encryption with customer-managed keys (SSE-C).
Please choose one or the other.`

type customEndpoint struct {
	Paths   []cty.Path
	EnvVars []string
}

func (e customEndpoint) Validate(obj cty.Value, diags *tfdiags.Diagnostics) {
	validateAttributesConflict(e.Paths...)(obj, cty.Path{}, diags)
}

func (e customEndpoint) String(obj cty.Value) string {
	v, _ := e.StringOk(obj)
	return v
}

func includeProtoIfNecessary(endpoint string) string {
	if matched, _ := regexp.MatchString("[a-z]*://.*", endpoint); !matched {
		log.Printf("[DEBUG] Adding https:// prefix to endpoint '%s'", endpoint)
		endpoint = fmt.Sprintf("https://%s", endpoint)
	}
	return endpoint
}

func (e customEndpoint) StringOk(obj cty.Value) (string, bool) {
	for _, path := range e.Paths {
		val, err := path.Apply(obj)
		if err != nil {
			continue
		}
		if s, ok := stringValueOk(val); ok {
			return includeProtoIfNecessary(s), true
		}
	}
	for _, envVar := range e.EnvVars {
		if v := os.Getenv(envVar); v != "" {
			return includeProtoIfNecessary(v), true
		}
	}
	return "", false
}

var customEndpoints = map[string]customEndpoint{
	"s3": {
		Paths: []cty.Path{
			cty.GetAttrPath("endpoints").GetAttr("s3"),
			cty.GetAttrPath("endpoint"),
		},
		EnvVars: []string{
			"AWS_ENDPOINT_URL_S3",
			"AWS_S3_ENDPOINT",
		},
	},
	"iam": {
		Paths: []cty.Path{
			cty.GetAttrPath("endpoints").GetAttr("iam"),
			cty.GetAttrPath("iam_endpoint"),
		},
		EnvVars: []string{
			"AWS_ENDPOINT_URL_IAM",
			"AWS_IAM_ENDPOINT",
		},
	},
	"sts": {
		Paths: []cty.Path{
			cty.GetAttrPath("endpoints").GetAttr("sts"),
			cty.GetAttrPath("sts_endpoint"),
		},
		EnvVars: []string{
			"AWS_ENDPOINT_URL_STS",
			"AWS_STS_ENDPOINT",
		},
	},
	"dynamodb": {
		Paths: []cty.Path{
			cty.GetAttrPath("endpoints").GetAttr("dynamodb"),
			cty.GetAttrPath("dynamodb_endpoint"),
		},
		EnvVars: []string{
			"AWS_ENDPOINT_URL_DYNAMODB",
			"AWS_DYNAMODB_ENDPOINT",
		},
	},
}
