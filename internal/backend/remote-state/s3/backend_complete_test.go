// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package s3

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hashicorp/aws-sdk-go-base/v2/mockdata"
	"github.com/hashicorp/aws-sdk-go-base/v2/servicemocks"
	"github.com/opentofu/opentofu/internal/configs/hcl2shim"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

const mockStsAssumeRolePolicy = `{
	"Version": "2012-10-17",
	"Statement": {
	  "Effect": "Allow",
	  "Action": "*",
	  "Resource": "*"
	}
  }`

func ExpectNoDiags(t *testing.T, diags tfdiags.Diagnostics) {
	expectDiagsCount(t, diags, 0)
}

func expectDiagsCount(t *testing.T, diags tfdiags.Diagnostics, c int) {
	if l := len(diags); l != c {
		t.Fatalf("Diagnostics: expected %d element, got %d\n%#v", c, l, diags)
	}
}

func ExpectDiagsEqual(expected tfdiags.Diagnostics) diagsValidator {
	return func(t *testing.T, diags tfdiags.Diagnostics) {
		if diff := cmp.Diff(diags, expected, cmp.Comparer(diagnosticComparer)); diff != "" {
			t.Fatalf("unexpected diagnostics difference: %s", diff)
		}
	}
}

type diagsValidator func(*testing.T, tfdiags.Diagnostics)

// ExpectDiagsMatching returns a validator expecting a single Diagnostic with fields matching the expectation
func ExpectDiagsMatching(severity tfdiags.Severity, summary matcher, detail matcher) diagsValidator {
	return func(t *testing.T, diags tfdiags.Diagnostics) {
		for _, d := range diags {
			if !summary.Match(d.Description().Summary) || !detail.Match(d.Description().Detail) {
				t.Fatalf("expected Diagnostic matching %#v, got %#v",
					tfdiags.Sourceless(
						severity,
						summary.String(),
						detail.String(),
					),
					d,
				)
			}
		}

		expectDiagsCount(t, diags, 1)
	}
}

type diagValidator func(*testing.T, tfdiags.Diagnostic)

func ExpectDiagMatching(severity tfdiags.Severity, summary matcher, detail matcher) diagValidator {
	return func(t *testing.T, d tfdiags.Diagnostic) {
		if !summary.Match(d.Description().Summary) || !detail.Match(d.Description().Detail) {
			t.Fatalf("expected Diagnostic matching %#v, got %#v",
				tfdiags.Sourceless(
					severity,
					summary.String(),
					detail.String(),
				),
				d,
			)
		}
	}
}

func ExpectMultipleDiags(validators ...diagValidator) diagsValidator {
	return func(t *testing.T, diags tfdiags.Diagnostics) {
		count := len(validators)
		if diagCount := len(diags); diagCount < count {
			count = diagCount
		}

		for i := 0; i < count; i++ {
			validators[i](t, diags[i])
		}

		expectDiagsCount(t, diags, len(validators))
	}
}

type matcher interface {
	fmt.Stringer
	Match(string) bool
}

type equalsMatcher string

func (m equalsMatcher) Match(s string) bool {
	return string(m) == s
}

func (m equalsMatcher) String() string {
	return string(m)
}

type regexpMatcher struct {
	re *regexp.Regexp
}

func newRegexpMatcher(re string) regexpMatcher {
	return regexpMatcher{
		re: regexp.MustCompile(re),
	}
}

func (m regexpMatcher) Match(s string) bool {
	return m.re.MatchString(s)
}

func (m regexpMatcher) String() string {
	return m.re.String()
}

type noopMatcher struct{}

func (m noopMatcher) Match(s string) bool {
	return true
}

func (m noopMatcher) String() string {
	return ""
}

func TestBackendConfig_Authentication(t *testing.T) {
	testCases := map[string]struct {
		config                     map[string]any
		EnableEc2MetadataServer    bool
		EnableEcsCredentialsServer bool
		EnableWebIdentityEnvVars   bool
		// EnableWebIdentityConfig    bool // Not supported
		EnvironmentVariables     map[string]string
		ExpectedCredentialsValue aws.Credentials
		MockStsEndpoints         []*servicemocks.MockEndpoint
		SharedConfigurationFile  string
		SharedCredentialsFile    string
		ValidateDiags            diagsValidator
	}{
		"empty config": {
			config: map[string]any{},
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			ValidateDiags: ExpectDiagsMatching(
				tfdiags.Error,
				equalsMatcher("No valid credential sources found"),
				newRegexpMatcher("^Please see.+"),
			),
		},

		"config AccessKey": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
			},
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			ExpectedCredentialsValue: mockdata.MockStaticCredentials,
			ValidateDiags:            ExpectNoDiags,
		},

		"config AccessKey forbidden account": {
			config: map[string]any{
				"access_key":            servicemocks.MockStaticAccessKey,
				"secret_key":            servicemocks.MockStaticSecretKey,
				"forbidden_account_ids": []any{"222222222222"},
			},
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			ExpectedCredentialsValue: mockdata.MockStaticCredentials,
			ValidateDiags: ExpectDiagsMatching(
				tfdiags.Error,
				equalsMatcher("Invalid account ID"),
				equalsMatcher("AWS account ID not allowed: 222222222222"),
			),
		},

		"config Profile shared credentials profile aws_access_key_id": {
			config: map[string]any{
				"profile": "SharedCredentialsProfile",
			},
			ExpectedCredentialsValue: aws.Credentials{
				AccessKeyID:     "ProfileSharedCredentialsAccessKey",
				SecretAccessKey: "ProfileSharedCredentialsSecretKey",
				Source:          "SharedConfigCredentials",
			},
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey

[SharedCredentialsProfile]
aws_access_key_id = ProfileSharedCredentialsAccessKey
aws_secret_access_key = ProfileSharedCredentialsSecretKey
`,
			ValidateDiags: ExpectNoDiags,
		},

		"environment AWS_ACCESS_KEY_ID does not override config Profile": {
			config: map[string]any{
				"profile": "SharedCredentialsProfile",
			},
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     servicemocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": servicemocks.MockEnvSecretKey,
			},
			ExpectedCredentialsValue: aws.Credentials{
				AccessKeyID:     "ProfileSharedCredentialsAccessKey",
				SecretAccessKey: "ProfileSharedCredentialsSecretKey",
				Source:          "SharedConfigCredentials",
			},
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
[SharedCredentialsProfile]
aws_access_key_id = ProfileSharedCredentialsAccessKey
aws_secret_access_key = ProfileSharedCredentialsSecretKey
`,
			ValidateDiags: ExpectNoDiags,
		},

		"environment AWS_ACCESS_KEY_ID": {
			config: map[string]any{},
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     servicemocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": servicemocks.MockEnvSecretKey,
			},
			ExpectedCredentialsValue: mockdata.MockEnvCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			ValidateDiags: ExpectNoDiags,
		},

		"environment AWS_PROFILE shared credentials profile aws_access_key_id": {
			config: map[string]any{},
			EnvironmentVariables: map[string]string{
				"AWS_PROFILE": "SharedCredentialsProfile",
			},
			ExpectedCredentialsValue: aws.Credentials{
				AccessKeyID:     "ProfileSharedCredentialsAccessKey",
				SecretAccessKey: "ProfileSharedCredentialsSecretKey",
				Source:          "SharedConfigCredentials",
			},
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey

[SharedCredentialsProfile]
aws_access_key_id = ProfileSharedCredentialsAccessKey
aws_secret_access_key = ProfileSharedCredentialsSecretKey
`,
			ValidateDiags: ExpectNoDiags,
		},

		"environment AWS_SESSION_TOKEN": {
			config: map[string]any{},
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     servicemocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": servicemocks.MockEnvSecretKey,
				"AWS_SESSION_TOKEN":     servicemocks.MockEnvSessionToken,
			},
			ExpectedCredentialsValue: mockdata.MockEnvCredentialsWithSessionToken,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},

		"shared credentials default aws_access_key_id": {
			config: map[string]any{},
			ExpectedCredentialsValue: aws.Credentials{
				AccessKeyID:     "DefaultSharedCredentialsAccessKey",
				SecretAccessKey: "DefaultSharedCredentialsSecretKey",
				Source:          "SharedConfigCredentials",
			},
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
		},

		"web identity token access key": {
			config:                   map[string]any{},
			EnableWebIdentityEnvVars: true,
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},

		"EC2 metadata access key": {
			config:                   map[string]any{},
			EnableEc2MetadataServer:  true,
			ExpectedCredentialsValue: mockdata.MockEc2MetadataCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			ValidateDiags: ExpectNoDiags,
		},

		"ECS credentials access key": {
			config:                     map[string]any{},
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue:   mockdata.MockEcsCredentialsCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},

		"AssumeWebIdentity envvar AssumeRoleARN access key": {
			config: map[string]any{
				"role_arn":     servicemocks.MockStsAssumeRoleArn,
				"session_name": servicemocks.MockStsAssumeRoleSessionName,
			},
			EnableWebIdentityEnvVars: true,
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			ValidateDiags: ExpectDiagsMatching(
				tfdiags.Warning,
				equalsMatcher("Deprecated Parameters"),
				noopMatcher{},
			),
		},

		"config AccessKey over environment AWS_ACCESS_KEY_ID": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     servicemocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": servicemocks.MockEnvSecretKey,
			},
			ExpectedCredentialsValue: mockdata.MockStaticCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			ValidateDiags: ExpectNoDiags,
		},

		"config AccessKey over shared credentials default aws_access_key_id": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
			},
			ExpectedCredentialsValue: mockdata.MockStaticCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
			ValidateDiags: ExpectNoDiags,
		},

		"config AccessKey over EC2 metadata access key": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
			},
			ExpectedCredentialsValue: mockdata.MockStaticCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},

		"config AccessKey over ECS credentials access key": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
			},
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue:   mockdata.MockStaticCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},

		"environment AWS_ACCESS_KEY_ID over shared credentials default aws_access_key_id": {
			config: map[string]any{},
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     servicemocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": servicemocks.MockEnvSecretKey,
			},
			ExpectedCredentialsValue: mockdata.MockEnvCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
			ValidateDiags: ExpectNoDiags,
		},

		"environment AWS_ACCESS_KEY_ID over EC2 metadata access key": {
			config: map[string]any{},
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     servicemocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": servicemocks.MockEnvSecretKey,
			},
			ExpectedCredentialsValue: mockdata.MockEnvCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},

		"environment AWS_ACCESS_KEY_ID over ECS credentials access key": {
			config: map[string]any{},
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     servicemocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": servicemocks.MockEnvSecretKey,
			},
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue:   mockdata.MockEnvCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},

		"shared credentials default aws_access_key_id over EC2 metadata access key": {
			config: map[string]any{},
			ExpectedCredentialsValue: aws.Credentials{
				AccessKeyID:     "DefaultSharedCredentialsAccessKey",
				SecretAccessKey: "DefaultSharedCredentialsSecretKey",
				Source:          "SharedConfigCredentials",
			},
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
		},

		"shared credentials default aws_access_key_id over ECS credentials access key": {
			config:                     map[string]any{},
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue: aws.Credentials{
				AccessKeyID:     "DefaultSharedCredentialsAccessKey",
				SecretAccessKey: "DefaultSharedCredentialsSecretKey",
				Source:          "SharedConfigCredentials",
			},
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
		},

		"ECS credentials access key over EC2 metadata access key": {
			config:                     map[string]any{},
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue:   mockdata.MockEcsCredentialsCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},

		"retrieve region from shared configuration file": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
			},
			ExpectedCredentialsValue: mockdata.MockStaticCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedConfigurationFile: `
[default]
region = us-east-1
`,
		},

		"skip EC2 Metadata API check": {
			config: map[string]any{
				"skip_metadata_api_check": true,
			},
			// The IMDS server must be enabled so that auth will succeed if the IMDS is called
			EnableEc2MetadataServer: true,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			ValidateDiags: ExpectDiagsMatching(
				tfdiags.Error,
				equalsMatcher("No valid credential sources found"),
				newRegexpMatcher("^Please see.+"),
			),
		},

		"invalid profile name from envvar": {
			config: map[string]any{},
			EnvironmentVariables: map[string]string{
				"AWS_PROFILE": "no-such-profile",
			},
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedCredentialsFile: `
[some-profile]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
			ValidateDiags: ExpectDiagsMatching(
				tfdiags.Error,
				equalsMatcher("failed to get shared config profile, no-such-profile"),
				equalsMatcher(""),
			),
		},

		"invalid profile name from config": {
			config: map[string]any{
				"profile": "no-such-profile",
			},
			SharedCredentialsFile: `
[some-profile]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			ValidateDiags: ExpectDiagsMatching(
				tfdiags.Error,
				equalsMatcher("failed to get shared config profile, no-such-profile"),
				equalsMatcher(""),
			),
		},

		"AWS_ACCESS_KEY_ID overrides AWS_PROFILE": {
			config: map[string]any{},
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     servicemocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": servicemocks.MockEnvSecretKey,
				"AWS_PROFILE":           "SharedCredentialsProfile",
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey

[SharedCredentialsProfile]
aws_access_key_id = ProfileSharedCredentialsAccessKey
aws_secret_access_key = ProfileSharedCredentialsSecretKey
`,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			ExpectedCredentialsValue: mockdata.MockEnvCredentials,
			ValidateDiags:            ExpectNoDiags,
		},

		"AWS_ACCESS_KEY_ID does not override invalid profile name from envvar": {
			config: map[string]any{},
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     servicemocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": servicemocks.MockEnvSecretKey,
				"AWS_PROFILE":           "no-such-profile",
			},
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedCredentialsFile: `
[some-profile]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
			ValidateDiags: ExpectDiagsMatching(
				tfdiags.Error,
				equalsMatcher("failed to get shared config profile, no-such-profile"),
				equalsMatcher(""),
			),
		},
	}

	for name, tc := range testCases {
		tc := tc

		t.Run(name, func(t *testing.T) {
			servicemocks.InitSessionTestEnv(t)

			// Populate required fields
			tc.config["region"] = "us-east-1"
			tc.config["bucket"] = "bucket"
			tc.config["key"] = "key"

			if tc.ValidateDiags == nil {
				tc.ValidateDiags = ExpectNoDiags
			}

			if tc.EnableEc2MetadataServer {
				closeEc2Metadata := servicemocks.AwsMetadataApiMock(append(
					servicemocks.Ec2metadata_securityCredentialsEndpoints,
					servicemocks.Ec2metadata_instanceIdEndpoint,
					servicemocks.Ec2metadata_iamInfoEndpoint,
				))
				defer closeEc2Metadata()
			}

			if tc.EnableEcsCredentialsServer {
				closeEcsCredentials := servicemocks.EcsCredentialsApiMock()
				defer closeEcsCredentials()
			}

			if tc.EnableWebIdentityEnvVars /*|| tc.EnableWebIdentityConfig*/ {
				file, err := os.CreateTemp("", "aws-sdk-go-base-web-identity-token-file")
				if err != nil {
					t.Fatalf("unexpected error creating temporary web identity token file: %s", err)
				}

				defer os.Remove(file.Name())

				err = os.WriteFile(file.Name(), []byte(servicemocks.MockWebIdentityToken), 0600)

				if err != nil {
					t.Fatalf("unexpected error writing web identity token file: %s", err)
				}

				if tc.EnableWebIdentityEnvVars {
					t.Setenv("AWS_ROLE_ARN", servicemocks.MockStsAssumeRoleWithWebIdentityArn)
					t.Setenv("AWS_ROLE_SESSION_NAME", servicemocks.MockStsAssumeRoleWithWebIdentitySessionName)
					t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", file.Name())
				} /*else if tc.EnableWebIdentityConfig {
					tc.Config.AssumeRoleWithWebIdentity = &AssumeRoleWithWebIdentity{
						RoleARN:              servicemocks.MockStsAssumeRoleWithWebIdentityArn,
						SessionName:          servicemocks.MockStsAssumeRoleWithWebIdentitySessionName,
						WebIdentityTokenFile: file.Name(),
					}
				}*/
			}

			ts := servicemocks.MockAwsApiServer("STS", tc.MockStsEndpoints)
			defer ts.Close()

			tc.config["sts_endpoint"] = ts.URL

			if tc.SharedConfigurationFile != "" {
				file, err := os.CreateTemp("", "aws-sdk-go-base-shared-configuration-file")

				if err != nil {
					t.Fatalf("unexpected error creating temporary shared configuration file: %s", err)
				}

				defer os.Remove(file.Name())

				err = os.WriteFile(file.Name(), []byte(tc.SharedConfigurationFile), 0600)

				if err != nil {
					t.Fatalf("unexpected error writing shared configuration file: %s", err)
				}

				setSharedConfigFile(t, file.Name())
			}

			if tc.SharedCredentialsFile != "" {
				file, err := os.CreateTemp("", "aws-sdk-go-base-shared-credentials-file")

				if err != nil {
					t.Fatalf("unexpected error creating temporary shared credentials file: %s", err)
				}

				defer os.Remove(file.Name())

				err = os.WriteFile(file.Name(), []byte(tc.SharedCredentialsFile), 0600)

				if err != nil {
					t.Fatalf("unexpected error writing shared credentials file: %s", err)
				}

				tc.config["shared_credentials_files"] = []any{file.Name()}
				if tc.ExpectedCredentialsValue.Source == "SharedConfigCredentials" {
					tc.ExpectedCredentialsValue.Source = fmt.Sprintf("SharedConfigCredentials: %s", file.Name())
				}

				tc.config["shared_config_files"] = []any{file.Name()}
			}

			for k, v := range tc.EnvironmentVariables {
				t.Setenv(k, v)
			}

			b, diags := configureBackend(t, tc.config)

			tc.ValidateDiags(t, diags)

			if diags.HasErrors() {
				return
			}

			credentials, err := b.awsConfig.Credentials.Retrieve(context.TODO())
			if err != nil {
				t.Fatalf("Error when requesting credentials: %s", err)
			}

			if diff := cmp.Diff(credentials, tc.ExpectedCredentialsValue, cmpopts.IgnoreFields(aws.Credentials{}, "Expires")); diff != "" {
				t.Fatalf("unexpected credentials: (- got, + expected)\n%s", diff)
			}
		})
	}
}
func TestBackendConfig_Authentication_AssumeRoleInline(t *testing.T) {
	testCases := map[string]struct {
		config                     map[string]any
		EnableEc2MetadataServer    bool
		EnableEcsCredentialsServer bool
		EnvironmentVariables       map[string]string
		ExpectedCredentialsValue   aws.Credentials
		MockStsEndpoints           []*servicemocks.MockEndpoint
		SharedConfigurationFile    string
		SharedCredentialsFile      string
		ValidateDiags              diagsValidator
	}{
		"from config access_key": {
			config: map[string]any{
				"access_key":   servicemocks.MockStaticAccessKey,
				"secret_key":   servicemocks.MockStaticSecretKey,
				"role_arn":     servicemocks.MockStsAssumeRoleArn,
				"session_name": servicemocks.MockStsAssumeRoleSessionName,
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			ValidateDiags: ExpectDiagsMatching(
				tfdiags.Warning,
				equalsMatcher("Deprecated Parameters"),
				noopMatcher{},
			),
		},

		"from environment AWS_ACCESS_KEY_ID": {
			config: map[string]any{
				"role_arn":     servicemocks.MockStsAssumeRoleArn,
				"session_name": servicemocks.MockStsAssumeRoleSessionName,
			},
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     servicemocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": servicemocks.MockEnvSecretKey,
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			ValidateDiags: ExpectDiagsMatching(
				tfdiags.Warning,
				equalsMatcher("Deprecated Parameters"),
				noopMatcher{},
			),
		},

		"from config Profile with Ec2InstanceMetadata source": {
			config: map[string]any{
				"profile": "SharedConfigurationProfile",
			},
			EnableEc2MetadataServer:  true,
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedConfigurationFile: fmt.Sprintf(`
[profile SharedConfigurationProfile]
credential_source = Ec2InstanceMetadata
role_arn = %[1]s
role_session_name = %[2]s
`, servicemocks.MockStsAssumeRoleArn, servicemocks.MockStsAssumeRoleSessionName),
		},

		"from environment AWS_PROFILE with Ec2InstanceMetadata source": {
			config:                  map[string]any{},
			EnableEc2MetadataServer: true,
			EnvironmentVariables: map[string]string{
				"AWS_PROFILE": "SharedConfigurationProfile",
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedConfigurationFile: fmt.Sprintf(`
[profile SharedConfigurationProfile]
credential_source = Ec2InstanceMetadata
role_arn = %[1]s
role_session_name = %[2]s
`, servicemocks.MockStsAssumeRoleArn, servicemocks.MockStsAssumeRoleSessionName),
		},

		"from config Profile with source profile": {
			config: map[string]any{
				"profile": "SharedConfigurationProfile",
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedConfigurationFile: fmt.Sprintf(`
[profile SharedConfigurationProfile]
role_arn = %[1]s
role_session_name = %[2]s
source_profile = SharedConfigurationSourceProfile

[profile SharedConfigurationSourceProfile]
aws_access_key_id = SharedConfigurationSourceAccessKey
aws_secret_access_key = SharedConfigurationSourceSecretKey
`, servicemocks.MockStsAssumeRoleArn, servicemocks.MockStsAssumeRoleSessionName),
		},

		"from environment AWS_PROFILE with source profile": {
			config: map[string]any{},
			EnvironmentVariables: map[string]string{
				"AWS_PROFILE": "SharedConfigurationProfile",
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedConfigurationFile: fmt.Sprintf(`
[profile SharedConfigurationProfile]
role_arn = %[1]s
role_session_name = %[2]s
source_profile = SharedConfigurationSourceProfile

[profile SharedConfigurationSourceProfile]
aws_access_key_id = SharedConfigurationSourceAccessKey
aws_secret_access_key = SharedConfigurationSourceSecretKey
`, servicemocks.MockStsAssumeRoleArn, servicemocks.MockStsAssumeRoleSessionName),
		},

		"from default profile": {
			config: map[string]any{
				"role_arn":     servicemocks.MockStsAssumeRoleArn,
				"session_name": servicemocks.MockStsAssumeRoleSessionName,
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
			ValidateDiags: ExpectDiagsMatching(
				tfdiags.Warning,
				equalsMatcher("Deprecated Parameters"),
				noopMatcher{},
			),
		},

		"from EC2 metadata": {
			config: map[string]any{
				"role_arn":     servicemocks.MockStsAssumeRoleArn,
				"session_name": servicemocks.MockStsAssumeRoleSessionName,
			},
			EnableEc2MetadataServer:  true,
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			ValidateDiags: ExpectDiagsMatching(
				tfdiags.Warning,
				equalsMatcher("Deprecated Parameters"),
				noopMatcher{},
			),
		},

		"from ECS credentials": {
			config: map[string]any{
				"role_arn":     servicemocks.MockStsAssumeRoleArn,
				"session_name": servicemocks.MockStsAssumeRoleSessionName,
			},
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue:   mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			ValidateDiags: ExpectDiagsMatching(
				tfdiags.Warning,
				equalsMatcher("Deprecated Parameters"),
				noopMatcher{},
			),
		},

		"with duration": {
			config: map[string]any{
				"access_key":                   servicemocks.MockStaticAccessKey,
				"secret_key":                   servicemocks.MockStaticSecretKey,
				"role_arn":                     servicemocks.MockStsAssumeRoleArn,
				"session_name":                 servicemocks.MockStsAssumeRoleSessionName,
				"assume_role_duration_seconds": 3600,
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"DurationSeconds": "3600"}),
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			ValidateDiags: ExpectDiagsMatching(
				tfdiags.Warning,
				equalsMatcher("Deprecated Parameters"),
				noopMatcher{},
			),
		},

		"with external ID": {
			config: map[string]any{
				"access_key":   servicemocks.MockStaticAccessKey,
				"secret_key":   servicemocks.MockStaticSecretKey,
				"role_arn":     servicemocks.MockStsAssumeRoleArn,
				"session_name": servicemocks.MockStsAssumeRoleSessionName,
				"external_id":  servicemocks.MockStsAssumeRoleExternalId,
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"ExternalId": servicemocks.MockStsAssumeRoleExternalId}),
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			ValidateDiags: ExpectDiagsMatching(
				tfdiags.Warning,
				equalsMatcher("Deprecated Parameters"),
				noopMatcher{},
			),
		},

		"with policy": {
			config: map[string]any{
				"access_key":         servicemocks.MockStaticAccessKey,
				"secret_key":         servicemocks.MockStaticSecretKey,
				"role_arn":           servicemocks.MockStsAssumeRoleArn,
				"session_name":       servicemocks.MockStsAssumeRoleSessionName,
				"assume_role_policy": mockStsAssumeRolePolicy,
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"Policy": mockStsAssumeRolePolicy}),
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			ValidateDiags: ExpectDiagsMatching(
				tfdiags.Warning,
				equalsMatcher("Deprecated Parameters"),
				noopMatcher{},
			),
		},

		"with policy ARNs": {
			config: map[string]any{
				"access_key":              servicemocks.MockStaticAccessKey,
				"secret_key":              servicemocks.MockStaticSecretKey,
				"role_arn":                servicemocks.MockStsAssumeRoleArn,
				"session_name":            servicemocks.MockStsAssumeRoleSessionName,
				"assume_role_policy_arns": []any{servicemocks.MockStsAssumeRolePolicyArn},
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"PolicyArns.member.1.arn": servicemocks.MockStsAssumeRolePolicyArn}),
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			ValidateDiags: ExpectDiagsMatching(
				tfdiags.Warning,
				equalsMatcher("Deprecated Parameters"),
				noopMatcher{},
			),
		},

		"with tags": {
			config: map[string]any{
				"access_key":   servicemocks.MockStaticAccessKey,
				"secret_key":   servicemocks.MockStaticSecretKey,
				"role_arn":     servicemocks.MockStsAssumeRoleArn,
				"session_name": servicemocks.MockStsAssumeRoleSessionName,
				"assume_role_tags": map[string]any{
					servicemocks.MockStsAssumeRoleTagKey: servicemocks.MockStsAssumeRoleTagValue,
				},
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"Tags.member.1.Key": servicemocks.MockStsAssumeRoleTagKey, "Tags.member.1.Value": servicemocks.MockStsAssumeRoleTagValue}),
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			ValidateDiags: ExpectDiagsMatching(
				tfdiags.Warning,
				equalsMatcher("Deprecated Parameters"),
				noopMatcher{},
			),
		},

		"with transitive tags": {
			config: map[string]any{
				"access_key":   servicemocks.MockStaticAccessKey,
				"secret_key":   servicemocks.MockStaticSecretKey,
				"role_arn":     servicemocks.MockStsAssumeRoleArn,
				"session_name": servicemocks.MockStsAssumeRoleSessionName,
				"assume_role_tags": map[string]any{
					servicemocks.MockStsAssumeRoleTagKey: servicemocks.MockStsAssumeRoleTagValue,
				},
				"assume_role_transitive_tag_keys": []any{servicemocks.MockStsAssumeRoleTagKey},
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"Tags.member.1.Key": servicemocks.MockStsAssumeRoleTagKey, "Tags.member.1.Value": servicemocks.MockStsAssumeRoleTagValue, "TransitiveTagKeys.member.1": servicemocks.MockStsAssumeRoleTagKey}),
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			ValidateDiags: ExpectDiagsMatching(
				tfdiags.Warning,
				equalsMatcher("Deprecated Parameters"),
				noopMatcher{},
			),
		},

		"error": {
			config: map[string]any{
				"access_key":   servicemocks.MockStaticAccessKey,
				"secret_key":   servicemocks.MockStaticSecretKey,
				"role_arn":     servicemocks.MockStsAssumeRoleArn,
				"session_name": servicemocks.MockStsAssumeRoleSessionName,
			},
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleInvalidEndpointInvalidClientTokenId,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			ValidateDiags: ExpectMultipleDiags(
				ExpectDiagMatching(
					tfdiags.Warning,
					equalsMatcher("Deprecated Parameters"),
					noopMatcher{},
				),
				ExpectDiagMatching(
					tfdiags.Error,
					equalsMatcher("Cannot assume IAM Role"),
					noopMatcher{},
				),
			),
		},
	}

	for name, tc := range testCases {
		tc := tc

		t.Run(name, func(t *testing.T) {
			servicemocks.InitSessionTestEnv(t)

			ctx := context.TODO()

			// Populate required fields
			tc.config["region"] = "us-east-1"
			tc.config["bucket"] = "bucket"
			tc.config["key"] = "key"

			if tc.ValidateDiags == nil {
				tc.ValidateDiags = ExpectNoDiags
			}

			if tc.EnableEc2MetadataServer {
				closeEc2Metadata := servicemocks.AwsMetadataApiMock(append(
					servicemocks.Ec2metadata_securityCredentialsEndpoints,
					servicemocks.Ec2metadata_instanceIdEndpoint,
					servicemocks.Ec2metadata_iamInfoEndpoint,
				))
				defer closeEc2Metadata()
			}

			if tc.EnableEcsCredentialsServer {
				closeEcsCredentials := servicemocks.EcsCredentialsApiMock()
				defer closeEcsCredentials()
			}

			ts := servicemocks.MockAwsApiServer("STS", tc.MockStsEndpoints)
			defer ts.Close()

			tc.config["sts_endpoint"] = ts.URL

			if tc.SharedConfigurationFile != "" {
				file, err := os.CreateTemp("", "aws-sdk-go-base-shared-configuration-file")

				if err != nil {
					t.Fatalf("unexpected error creating temporary shared configuration file: %s", err)
				}

				defer os.Remove(file.Name())

				err = os.WriteFile(file.Name(), []byte(tc.SharedConfigurationFile), 0600)

				if err != nil {
					t.Fatalf("unexpected error writing shared configuration file: %s", err)
				}

				setSharedConfigFile(t, file.Name())
			}

			if tc.SharedCredentialsFile != "" {
				file, err := os.CreateTemp("", "aws-sdk-go-base-shared-credentials-file")

				if err != nil {
					t.Fatalf("unexpected error creating temporary shared credentials file: %s", err)
				}

				defer os.Remove(file.Name())

				err = os.WriteFile(file.Name(), []byte(tc.SharedCredentialsFile), 0600)

				if err != nil {
					t.Fatalf("unexpected error writing shared credentials file: %s", err)
				}

				tc.config["shared_credentials_files"] = []any{file.Name()}
				if tc.ExpectedCredentialsValue.Source == "SharedConfigCredentials" {
					tc.ExpectedCredentialsValue.Source = fmt.Sprintf("SharedConfigCredentials: %s", file.Name())
				}
			}

			for k, v := range tc.EnvironmentVariables {
				t.Setenv(k, v)
			}

			b, diags := configureBackend(t, tc.config)

			tc.ValidateDiags(t, diags)

			if diags.HasErrors() {
				return
			}

			credentials, err := b.awsConfig.Credentials.Retrieve(ctx)
			if err != nil {
				t.Fatalf("Error when requesting credentials: %s", err)
			}

			if diff := cmp.Diff(credentials, tc.ExpectedCredentialsValue, cmpopts.IgnoreFields(aws.Credentials{}, "Expires")); diff != "" {
				t.Fatalf("unexpected credentials: (- got, + expected)\n%s", diff)
			}
		})
	}
}

func TestBackendConfig_Authentication_AssumeRoleNested(t *testing.T) {
	testCases := map[string]struct {
		config                     map[string]any
		EnableEc2MetadataServer    bool
		EnableEcsCredentialsServer bool
		EnvironmentVariables       map[string]string
		ExpectedCredentialsValue   aws.Credentials
		MockStsEndpoints           []*servicemocks.MockEndpoint
		SharedConfigurationFile    string
		SharedCredentialsFile      string
		ValidateDiags              diagsValidator
	}{
		"from config access_key": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
				"assume_role": map[string]any{
					"role_arn":     servicemocks.MockStsAssumeRoleArn,
					"session_name": servicemocks.MockStsAssumeRoleSessionName,
				},
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},

		"from environment AWS_ACCESS_KEY_ID": {
			config: map[string]any{
				"assume_role": map[string]any{
					"role_arn":     servicemocks.MockStsAssumeRoleArn,
					"session_name": servicemocks.MockStsAssumeRoleSessionName,
				},
			},
			EnvironmentVariables: map[string]string{
				"AWS_ACCESS_KEY_ID":     servicemocks.MockEnvAccessKey,
				"AWS_SECRET_ACCESS_KEY": servicemocks.MockEnvSecretKey,
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},

		"from config Profile with Ec2InstanceMetadata source": {
			config: map[string]any{
				"profile": "SharedConfigurationProfile",
			},
			EnableEc2MetadataServer:  true,
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedConfigurationFile: fmt.Sprintf(`
[profile SharedConfigurationProfile]
credential_source = Ec2InstanceMetadata
role_arn = %[1]s
role_session_name = %[2]s
`, servicemocks.MockStsAssumeRoleArn, servicemocks.MockStsAssumeRoleSessionName),
		},

		"from environment AWS_PROFILE with Ec2InstanceMetadata source": {
			config:                  map[string]any{},
			EnableEc2MetadataServer: true,
			EnvironmentVariables: map[string]string{
				"AWS_PROFILE": "SharedConfigurationProfile",
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedConfigurationFile: fmt.Sprintf(`
[profile SharedConfigurationProfile]
credential_source = Ec2InstanceMetadata
role_arn = %[1]s
role_session_name = %[2]s
`, servicemocks.MockStsAssumeRoleArn, servicemocks.MockStsAssumeRoleSessionName),
		},

		"from config Profile with source profile": {
			config: map[string]any{
				"profile": "SharedConfigurationProfile",
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedConfigurationFile: fmt.Sprintf(`
[profile SharedConfigurationProfile]
role_arn = %[1]s
role_session_name = %[2]s
source_profile = SharedConfigurationSourceProfile

[profile SharedConfigurationSourceProfile]
aws_access_key_id = SharedConfigurationSourceAccessKey
aws_secret_access_key = SharedConfigurationSourceSecretKey
`, servicemocks.MockStsAssumeRoleArn, servicemocks.MockStsAssumeRoleSessionName),
		},

		"from environment AWS_PROFILE with source profile": {
			config: map[string]any{},
			EnvironmentVariables: map[string]string{
				"AWS_PROFILE": "SharedConfigurationProfile",
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedConfigurationFile: fmt.Sprintf(`
[profile SharedConfigurationProfile]
role_arn = %[1]s
role_session_name = %[2]s
source_profile = SharedConfigurationSourceProfile

[profile SharedConfigurationSourceProfile]
aws_access_key_id = SharedConfigurationSourceAccessKey
aws_secret_access_key = SharedConfigurationSourceSecretKey
`, servicemocks.MockStsAssumeRoleArn, servicemocks.MockStsAssumeRoleSessionName),
		},

		"from default profile": {
			config: map[string]any{
				"assume_role": map[string]any{
					"role_arn":     servicemocks.MockStsAssumeRoleArn,
					"session_name": servicemocks.MockStsAssumeRoleSessionName,
				},
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			SharedCredentialsFile: `
[default]
aws_access_key_id = DefaultSharedCredentialsAccessKey
aws_secret_access_key = DefaultSharedCredentialsSecretKey
`,
		},

		"from EC2 metadata": {
			config: map[string]any{
				"assume_role": map[string]any{
					"role_arn":     servicemocks.MockStsAssumeRoleArn,
					"session_name": servicemocks.MockStsAssumeRoleSessionName,
				},
			},
			EnableEc2MetadataServer:  true,
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},

		"from ECS credentials": {
			config: map[string]any{
				"assume_role": map[string]any{
					"role_arn":     servicemocks.MockStsAssumeRoleArn,
					"session_name": servicemocks.MockStsAssumeRoleSessionName,
				},
			},
			EnableEcsCredentialsServer: true,
			ExpectedCredentialsValue:   mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpoint,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},

		"with duration": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
				"assume_role": map[string]any{
					"role_arn":     servicemocks.MockStsAssumeRoleArn,
					"session_name": servicemocks.MockStsAssumeRoleSessionName,
					"duration":     "1h",
				},
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"DurationSeconds": "3600"}),
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},

		"with external ID": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
				"assume_role": map[string]any{
					"role_arn":     servicemocks.MockStsAssumeRoleArn,
					"session_name": servicemocks.MockStsAssumeRoleSessionName,
					"external_id":  servicemocks.MockStsAssumeRoleExternalId,
				},
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"ExternalId": servicemocks.MockStsAssumeRoleExternalId}),
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},

		"with policy": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
				"assume_role": map[string]any{
					"role_arn":     servicemocks.MockStsAssumeRoleArn,
					"session_name": servicemocks.MockStsAssumeRoleSessionName,
					"policy":       mockStsAssumeRolePolicy,
				},
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"Policy": mockStsAssumeRolePolicy}),
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},

		"with policy ARNs": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
				"assume_role": map[string]any{
					"role_arn":     servicemocks.MockStsAssumeRoleArn,
					"session_name": servicemocks.MockStsAssumeRoleSessionName,
					"policy_arns":  []any{servicemocks.MockStsAssumeRolePolicyArn},
				},
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"PolicyArns.member.1.arn": servicemocks.MockStsAssumeRolePolicyArn}),
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},

		"with tags": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
				"assume_role": map[string]any{
					"role_arn":     servicemocks.MockStsAssumeRoleArn,
					"session_name": servicemocks.MockStsAssumeRoleSessionName,
					"tags": map[string]any{
						servicemocks.MockStsAssumeRoleTagKey: servicemocks.MockStsAssumeRoleTagValue,
					},
				},
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"Tags.member.1.Key": servicemocks.MockStsAssumeRoleTagKey, "Tags.member.1.Value": servicemocks.MockStsAssumeRoleTagValue}),
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},

		"with transitive tags": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
				"assume_role": map[string]any{
					"role_arn":     servicemocks.MockStsAssumeRoleArn,
					"session_name": servicemocks.MockStsAssumeRoleSessionName,
					"tags": map[string]any{
						servicemocks.MockStsAssumeRoleTagKey: servicemocks.MockStsAssumeRoleTagValue,
					},
					"transitive_tag_keys": []any{servicemocks.MockStsAssumeRoleTagKey},
				},
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleValidEndpointWithOptions(map[string]string{"Tags.member.1.Key": servicemocks.MockStsAssumeRoleTagKey, "Tags.member.1.Value": servicemocks.MockStsAssumeRoleTagValue, "TransitiveTagKeys.member.1": servicemocks.MockStsAssumeRoleTagKey}),
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
		},

		"error": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
				"assume_role": map[string]any{
					"role_arn":     servicemocks.MockStsAssumeRoleArn,
					"session_name": servicemocks.MockStsAssumeRoleSessionName,
				},
			},
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleInvalidEndpointInvalidClientTokenId,
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			},
			ValidateDiags: ExpectDiagsMatching(
				tfdiags.Error,
				equalsMatcher("Cannot assume IAM Role"),
				noopMatcher{},
			),
		},
	}

	for name, tc := range testCases {
		tc := tc

		t.Run(name, func(t *testing.T) {
			servicemocks.InitSessionTestEnv(t)

			ctx := context.TODO()

			// Populate required fields
			tc.config["region"] = "us-east-1"
			tc.config["bucket"] = "bucket"
			tc.config["key"] = "key"

			if tc.ValidateDiags == nil {
				tc.ValidateDiags = ExpectNoDiags
			}

			if tc.EnableEc2MetadataServer {
				closeEc2Metadata := servicemocks.AwsMetadataApiMock(append(
					servicemocks.Ec2metadata_securityCredentialsEndpoints,
					servicemocks.Ec2metadata_instanceIdEndpoint,
					servicemocks.Ec2metadata_iamInfoEndpoint,
				))
				defer closeEc2Metadata()
			}

			if tc.EnableEcsCredentialsServer {
				closeEcsCredentials := servicemocks.EcsCredentialsApiMock()
				defer closeEcsCredentials()
			}

			ts := servicemocks.MockAwsApiServer("STS", tc.MockStsEndpoints)
			defer ts.Close()

			tc.config["sts_endpoint"] = ts.URL

			if tc.SharedConfigurationFile != "" {
				file, err := os.CreateTemp("", "aws-sdk-go-base-shared-configuration-file")

				if err != nil {
					t.Fatalf("unexpected error creating temporary shared configuration file: %s", err)
				}

				defer os.Remove(file.Name())

				err = os.WriteFile(file.Name(), []byte(tc.SharedConfigurationFile), 0600)

				if err != nil {
					t.Fatalf("unexpected error writing shared configuration file: %s", err)
				}

				setSharedConfigFile(t, file.Name())
			}

			if tc.SharedCredentialsFile != "" {
				file, err := os.CreateTemp("", "aws-sdk-go-base-shared-credentials-file")

				if err != nil {
					t.Fatalf("unexpected error creating temporary shared credentials file: %s", err)
				}

				defer os.Remove(file.Name())

				err = os.WriteFile(file.Name(), []byte(tc.SharedCredentialsFile), 0600)

				if err != nil {
					t.Fatalf("unexpected error writing shared credentials file: %s", err)
				}

				tc.config["shared_credentials_files"] = []any{file.Name()}
				if tc.ExpectedCredentialsValue.Source == "SharedConfigCredentials" {
					tc.ExpectedCredentialsValue.Source = fmt.Sprintf("SharedConfigCredentials: %s", file.Name())
				}

				tc.config["shared_config_files"] = []any{file.Name()}
			}

			for k, v := range tc.EnvironmentVariables {
				t.Setenv(k, v)
			}

			b, diags := configureBackend(t, tc.config)

			tc.ValidateDiags(t, diags)

			if diags.HasErrors() {
				return
			}

			credentials, err := b.awsConfig.Credentials.Retrieve(ctx)
			if err != nil {
				t.Fatalf("Error when requesting credentials: %s", err)
			}

			if diff := cmp.Diff(credentials, tc.ExpectedCredentialsValue, cmpopts.IgnoreFields(aws.Credentials{}, "Expires")); diff != "" {
				t.Fatalf("unexpected credentials: (- got, + expected)\n%s", diff)
			}
		})
	}
}

func TestBackendConfig_Authentication_AssumeRoleWithWebIdentity(t *testing.T) {
	testCases := map[string]struct {
		config                          map[string]any
		SetConfig                       bool
		ExpandEnvVars                   bool
		EnvironmentVariables            map[string]string
		SetTokenFileEnvironmentVariable bool
		SharedConfigurationFile         string
		SetSharedConfigurationFile      bool
		ExpectedCredentialsValue        aws.Credentials
		ValidateDiags                   diagsValidator
		MockStsEndpoints                []*servicemocks.MockEndpoint
	}{
		"config with inline token": {
			config: map[string]any{
				"assume_role_with_web_identity": map[string]any{
					"role_arn":           servicemocks.MockStsAssumeRoleWithWebIdentityArn,
					"session_name":       servicemocks.MockStsAssumeRoleWithWebIdentitySessionName,
					"web_identity_token": servicemocks.MockWebIdentityToken,
				},
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
			},
		},

		"config with token file": {
			config: map[string]any{
				"assume_role_with_web_identity": map[string]any{
					"role_arn":     servicemocks.MockStsAssumeRoleWithWebIdentityArn,
					"session_name": servicemocks.MockStsAssumeRoleWithWebIdentitySessionName,
				},
			},
			SetConfig:                true,
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
			},
		},

		"config with expanded path": {
			config: map[string]any{
				"assume_role_with_web_identity": map[string]any{
					"role_arn":     servicemocks.MockStsAssumeRoleWithWebIdentityArn,
					"session_name": servicemocks.MockStsAssumeRoleWithWebIdentitySessionName,
				},
			},
			SetConfig:                true,
			ExpandEnvVars:            true,
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
			},
		},

		"envvar": {
			config: map[string]any{},
			EnvironmentVariables: map[string]string{
				"AWS_ROLE_ARN":          servicemocks.MockStsAssumeRoleWithWebIdentityArn,
				"AWS_ROLE_SESSION_NAME": servicemocks.MockStsAssumeRoleWithWebIdentitySessionName,
			},
			SetTokenFileEnvironmentVariable: true,
			ExpectedCredentialsValue:        mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
			},
		},

		"shared configuration file": {
			config: map[string]any{},
			SharedConfigurationFile: fmt.Sprintf(`
[default]
role_arn = %[1]s
role_session_name = %[2]s
`, servicemocks.MockStsAssumeRoleWithWebIdentityArn, servicemocks.MockStsAssumeRoleWithWebIdentitySessionName),
			SetSharedConfigurationFile: true,
			ExpectedCredentialsValue:   mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
			},
		},

		"config overrides envvar": {
			config: map[string]any{
				"assume_role_with_web_identity": map[string]any{
					"role_arn":           servicemocks.MockStsAssumeRoleWithWebIdentityArn,
					"session_name":       servicemocks.MockStsAssumeRoleWithWebIdentitySessionName,
					"web_identity_token": servicemocks.MockWebIdentityToken,
				},
			},
			EnvironmentVariables: map[string]string{
				"AWS_ROLE_ARN":                servicemocks.MockStsAssumeRoleWithWebIdentityAlternateArn,
				"AWS_ROLE_SESSION_NAME":       servicemocks.MockStsAssumeRoleWithWebIdentityAlternateSessionName,
				"AWS_WEB_IDENTITY_TOKEN_FILE": "no-such-file",
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
			},
		},

		"envvar overrides shared configuration": {
			config: map[string]any{},
			EnvironmentVariables: map[string]string{
				"AWS_ROLE_ARN":          servicemocks.MockStsAssumeRoleWithWebIdentityArn,
				"AWS_ROLE_SESSION_NAME": servicemocks.MockStsAssumeRoleWithWebIdentitySessionName,
			},
			SetTokenFileEnvironmentVariable: true,
			SharedConfigurationFile: fmt.Sprintf(`
[default]
role_arn = %[1]s
role_session_name = %[2]s
web_identity_token_file = no-such-file
`, servicemocks.MockStsAssumeRoleWithWebIdentityAlternateArn, servicemocks.MockStsAssumeRoleWithWebIdentityAlternateSessionName),
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
			},
		},

		"config overrides shared configuration": {
			config: map[string]any{
				"assume_role_with_web_identity": map[string]any{
					"role_arn":           servicemocks.MockStsAssumeRoleWithWebIdentityArn,
					"session_name":       servicemocks.MockStsAssumeRoleWithWebIdentitySessionName,
					"web_identity_token": servicemocks.MockWebIdentityToken,
				},
			},
			SharedConfigurationFile: fmt.Sprintf(`
[default]
role_arn = %[1]s
role_session_name = %[2]s
web_identity_token_file = no-such-file
`, servicemocks.MockStsAssumeRoleWithWebIdentityAlternateArn, servicemocks.MockStsAssumeRoleWithWebIdentityAlternateSessionName),
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleWithWebIdentityValidEndpoint,
			},
		},

		"with duration": {
			config: map[string]any{
				"assume_role_with_web_identity": map[string]any{
					"role_arn":           servicemocks.MockStsAssumeRoleWithWebIdentityArn,
					"session_name":       servicemocks.MockStsAssumeRoleWithWebIdentitySessionName,
					"web_identity_token": servicemocks.MockWebIdentityToken,
					"duration":           "1h",
				},
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleWithWebIdentityValidWithOptions(map[string]string{"DurationSeconds": "3600"}),
			},
		},

		"with policy": {
			config: map[string]any{
				"assume_role_with_web_identity": map[string]any{
					"role_arn":           servicemocks.MockStsAssumeRoleWithWebIdentityArn,
					"session_name":       servicemocks.MockStsAssumeRoleWithWebIdentitySessionName,
					"web_identity_token": servicemocks.MockWebIdentityToken,
					"policy":             "{}",
				},
			},
			ExpectedCredentialsValue: mockdata.MockStsAssumeRoleWithWebIdentityCredentials,
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				servicemocks.MockStsAssumeRoleWithWebIdentityValidWithOptions(map[string]string{"Policy": "{}"}),
			},
		},
	}

	for name, tc := range testCases {
		tc := tc

		t.Run(name, func(t *testing.T) {
			servicemocks.InitSessionTestEnv(t)

			ctx := context.TODO()

			// Populate required fields
			tc.config["region"] = "us-east-1"
			tc.config["bucket"] = "bucket"
			tc.config["key"] = "key"

			if tc.ValidateDiags == nil {
				tc.ValidateDiags = ExpectNoDiags
			}

			for k, v := range tc.EnvironmentVariables {
				t.Setenv(k, v)
			}

			ts := servicemocks.MockAwsApiServer("STS", tc.MockStsEndpoints)
			defer ts.Close()

			tc.config["sts_endpoint"] = ts.URL

			t.Setenv("TMPDIR", t.TempDir())

			tokenFile, err := os.CreateTemp("", "aws-sdk-go-base-web-identity-token-file")
			if err != nil {
				t.Fatalf("unexpected error creating temporary web identity token file: %s", err)
			}
			tokenFileName := tokenFile.Name()

			defer os.Remove(tokenFileName)

			err = os.WriteFile(tokenFileName, []byte(servicemocks.MockWebIdentityToken), 0600)

			if err != nil {
				t.Fatalf("unexpected error writing web identity token file: %s", err)
			}

			if tc.ExpandEnvVars {
				tmpdir := os.Getenv("TMPDIR")
				rel, err := filepath.Rel(tmpdir, tokenFileName)
				if err != nil {
					t.Fatalf("error making path relative: %s", err)
				}
				t.Logf("relative: %s", rel)
				tokenFileName = filepath.Join("$TMPDIR", rel)
				t.Logf("env tempfile: %s", tokenFileName)
			}

			if tc.SetConfig {
				ar := tc.config["assume_role_with_web_identity"].(map[string]any)
				ar["web_identity_token_file"] = tokenFileName
			}

			if tc.SetTokenFileEnvironmentVariable {
				t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", tokenFileName)
			}

			if tc.SharedConfigurationFile != "" {
				file, err := os.CreateTemp("", "aws-sdk-go-base-shared-configuration-file")

				if err != nil {
					t.Fatalf("unexpected error creating temporary shared configuration file: %s", err)
				}

				defer os.Remove(file.Name())

				if tc.SetSharedConfigurationFile {
					tc.SharedConfigurationFile += fmt.Sprintf("web_identity_token_file = %s\n", tokenFileName)
				}

				err = os.WriteFile(file.Name(), []byte(tc.SharedConfigurationFile), 0600)

				if err != nil {
					t.Fatalf("unexpected error writing shared configuration file: %s", err)
				}

				tc.config["shared_config_files"] = []any{file.Name()}
			}

			tc.config["skip_credentials_validation"] = true

			b, diags := configureBackend(t, tc.config)

			tc.ValidateDiags(t, diags)

			if diags.HasErrors() {
				return
			}

			credentials, err := b.awsConfig.Credentials.Retrieve(ctx)
			if err != nil {
				t.Fatalf("Error when requesting credentials: %s", err)
			}

			if diff := cmp.Diff(credentials, tc.ExpectedCredentialsValue, cmpopts.IgnoreFields(aws.Credentials{}, "Expires")); diff != "" {
				t.Fatalf("unexpected credentials: (- got, + expected)\n%s", diff)
			}
		})
	}
}

func TestBackendConfig_Region(t *testing.T) {
	testCases := map[string]struct {
		config                  map[string]any
		EnvironmentVariables    map[string]string
		IMDSRegion              string
		SharedConfigurationFile string
		ExpectedRegion          string
	}{
		// NOT SUPPORTED: region is required
		// "no configuration": {
		// 	config: map[string]any{
		// 		"access_key": servicemocks.MockStaticAccessKey,
		// 		"secret_key": servicemocks.MockStaticSecretKey,
		// 	},
		// 	ExpectedRegion: "",
		// },

		"config": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
				"region":     "us-east-1",
			},
			ExpectedRegion: "us-east-1",
		},

		"AWS_REGION": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_REGION": "us-east-1",
			},
			ExpectedRegion: "us-east-1",
		},
		"AWS_DEFAULT_REGION": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_DEFAULT_REGION": "us-east-1",
			},
			ExpectedRegion: "us-east-1",
		},
		"AWS_REGION overrides AWS_DEFAULT_REGION": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_REGION":         "us-east-1",
				"AWS_DEFAULT_REGION": "us-west-2",
			},
			ExpectedRegion: "us-east-1",
		},

		// NOT SUPPORTED: region from shared configuration file
		// 		"shared configuration file": {
		// 			config: map[string]any{
		// 				"access_key": servicemocks.MockStaticAccessKey,
		// 				"secret_key": servicemocks.MockStaticSecretKey,
		// 			},
		// 			SharedConfigurationFile: `
		// [default]
		// region = us-east-1
		// `,
		// 			ExpectedRegion: "us-east-1",
		// 		},

		// NOT SUPPORTED: region from IMDS
		// "IMDS": {
		// 	config:         map[string]any{},
		// 	IMDSRegion:     "us-east-1",
		// 	ExpectedRegion: "us-east-1",
		// },

		"config overrides AWS_REGION": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
				"region":     "us-east-1",
			},
			EnvironmentVariables: map[string]string{
				"AWS_REGION": "us-west-2",
			},
			ExpectedRegion: "us-east-1",
		},
		"config overrides AWS_DEFAULT_REGION": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
				"region":     "us-east-1",
			},
			EnvironmentVariables: map[string]string{
				"AWS_DEFAULT_REGION": "us-west-2",
			},
			ExpectedRegion: "us-east-1",
		},

		"config overrides IMDS": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
				"region":     "us-west-2",
			},
			IMDSRegion:     "us-east-1",
			ExpectedRegion: "us-west-2",
		},

		"AWS_REGION overrides shared configuration": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_REGION": "us-east-1",
			},
			SharedConfigurationFile: `
[default]
region = us-west-2
`,
			ExpectedRegion: "us-east-1",
		},
		"AWS_DEFAULT_REGION overrides shared configuration": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_DEFAULT_REGION": "us-east-1",
			},
			SharedConfigurationFile: `
[default]
region = us-west-2
`,
			ExpectedRegion: "us-east-1",
		},

		"AWS_REGION overrides IMDS": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_REGION": "us-east-1",
			},
			IMDSRegion:     "us-west-2",
			ExpectedRegion: "us-east-1",
		},
	}

	for name, tc := range testCases {
		tc := tc

		t.Run(name, func(t *testing.T) {
			servicemocks.InitSessionTestEnv(t)

			// Populate required fields
			tc.config["bucket"] = "bucket"
			tc.config["key"] = "key"

			for k, v := range tc.EnvironmentVariables {
				t.Setenv(k, v)
			}

			if tc.IMDSRegion != "" {
				closeEc2Metadata := servicemocks.AwsMetadataApiMock(append(
					servicemocks.Ec2metadata_securityCredentialsEndpoints,
					servicemocks.Ec2metadata_instanceIdEndpoint,
					servicemocks.Ec2metadata_iamInfoEndpoint,
					servicemocks.Ec2metadata_instanceIdentityEndpoint(tc.IMDSRegion),
				))
				defer closeEc2Metadata()
			}

			sts := servicemocks.MockAwsApiServer("STS", []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			})
			defer sts.Close()

			tc.config["sts_endpoint"] = sts.URL

			if tc.SharedConfigurationFile != "" {
				file, err := os.CreateTemp("", "aws-sdk-go-base-shared-configuration-file")

				if err != nil {
					t.Fatalf("unexpected error creating temporary shared configuration file: %s", err)
				}

				defer os.Remove(file.Name())

				err = os.WriteFile(file.Name(), []byte(tc.SharedConfigurationFile), 0600)

				if err != nil {
					t.Fatalf("unexpected error writing shared configuration file: %s", err)
				}

				setSharedConfigFile(t, file.Name())
			}

			tc.config["skip_credentials_validation"] = true

			b, diags := configureBackend(t, tc.config)
			if diags.HasErrors() {
				t.Fatalf("configuring backend: %s", diagnosticsString(diags))
			}

			if a, e := b.awsConfig.Region, tc.ExpectedRegion; a != e {
				t.Errorf("expected Region %q, got: %q", e, a)
			}
		})
	}
}

func TestBackendConfig_RetryMode(t *testing.T) {
	testCases := map[string]struct {
		config               map[string]any
		EnvironmentVariables map[string]string
		ExpectedMode         aws.RetryMode
	}{
		"no config": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
			},
			ExpectedMode: "",
		},

		"config": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
				"retry_mode": "standard",
			},
			ExpectedMode: aws.RetryModeStandard,
		},

		"AWS_RETRY_MODE": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
			},
			EnvironmentVariables: map[string]string{
				"AWS_RETRY_MODE": "adaptive",
			},
			ExpectedMode: aws.RetryModeAdaptive,
		},
		"config overrides AWS_RETRY_MODE": {
			config: map[string]any{
				"access_key": servicemocks.MockStaticAccessKey,
				"secret_key": servicemocks.MockStaticSecretKey,
				"retry_mode": "standard",
			},
			EnvironmentVariables: map[string]string{
				"AWS_RETRY_MODE": "adaptive",
			},
			ExpectedMode: aws.RetryModeStandard,
		},
	}

	for name, tc := range testCases {
		tc := tc

		t.Run(name, func(t *testing.T) {
			servicemocks.InitSessionTestEnv(t)

			// Populate required fields
			tc.config["bucket"] = "bucket"
			tc.config["key"] = "key"
			tc.config["region"] = "us-east-1"

			for k, v := range tc.EnvironmentVariables {
				t.Setenv(k, v)
			}

			sts := servicemocks.MockAwsApiServer("STS", []*servicemocks.MockEndpoint{
				servicemocks.MockStsGetCallerIdentityValidEndpoint,
			})
			defer sts.Close()

			tc.config["sts_endpoint"] = sts.URL
			tc.config["skip_credentials_validation"] = true

			b, diags := configureBackend(t, tc.config)
			if diags.HasErrors() {
				t.Fatalf("configuring backend: %s", diagnosticsString(diags))
			}

			if a, e := b.awsConfig.RetryMode, tc.ExpectedMode; a != e {
				t.Errorf("expected mode %q, got: %q", e, a)
			}
		})
	}
}

func setSharedConfigFile(t *testing.T, filename string) {
	t.Helper()
	t.Setenv("AWS_SDK_LOAD_CONFIG", "1")
	t.Setenv("AWS_CONFIG_FILE", filename)
}

func configureBackend(t *testing.T, config map[string]any) (*Backend, tfdiags.Diagnostics) {
	b := New(encryption.StateEncryptionDisabled()).(*Backend)
	configSchema := populateSchema(t, b.ConfigSchema(), hcl2shim.HCL2ValueFromConfigValue(config))

	configSchema, diags := b.PrepareConfig(configSchema)

	if diags.HasErrors() {
		return b, diags
	}

	confDiags := b.Configure(configSchema)
	diags = diags.Append(confDiags)

	return b, diags
}
