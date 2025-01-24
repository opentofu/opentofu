// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package s3

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/aws-sdk-go-base/v2/mockdata"
	"github.com/hashicorp/aws-sdk-go-base/v2/servicemocks"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/configs/hcl2shim"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

const testBucketPrefix = "tofu-test"

var (
	mockStsGetCallerIdentityRequestBody = url.Values{
		"Action":  []string{"GetCallerIdentity"},
		"Version": []string{"2011-06-15"},
	}.Encode()
)

// verify that we are doing ACC tests or the S3 tests specifically
func testACC(t *testing.T) {
	t.Helper()
	skip := os.Getenv("TF_ACC") == "" && os.Getenv("TF_S3_TEST") == ""
	if skip {
		t.Log("s3 backend tests require setting TF_ACC or TF_S3_TEST")
		t.Skip()
	}
	t.Setenv("AWS_DEFAULT_REGION", "us-west-2")
}

func TestBackend_impl(t *testing.T) {
	var _ backend.Backend = new(Backend)
}

func TestBackendConfig_original(t *testing.T) {
	testACC(t)
	config := map[string]interface{}{
		"region":         "us-west-1",
		"bucket":         testBucketPrefix,
		"key":            "state",
		"encrypt":        true,
		"dynamodb_table": "dynamoTable",
	}

	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(config)).(*Backend)

	if b.awsConfig.Region != "us-west-1" {
		t.Fatalf("Incorrect region was populated")
	}
	if b.awsConfig.RetryMaxAttempts != 5 {
		t.Fatalf("Default max_retries was not set")
	}
	if b.bucketName != testBucketPrefix {
		t.Fatalf("Incorrect bucketName was populated")
	}
	if b.keyName != "state" {
		t.Fatalf("Incorrect keyName was populated")
	}

	credentials, err := b.awsConfig.Credentials.Retrieve(context.TODO())
	if err != nil {
		t.Fatalf("Error when requesting credentials")
	}
	if credentials.AccessKeyID == "" {
		t.Fatalf("No Access Key Id was populated")
	}
	if credentials.SecretAccessKey == "" {
		t.Fatalf("No Secret Access Key was populated")
	}
}

func TestBackendConfig_InvalidRegion(t *testing.T) {
	testACC(t)

	cases := map[string]struct {
		config        map[string]any
		expectedDiags tfdiags.Diagnostics
	}{
		"with region validation": {
			config: map[string]interface{}{
				"region":                      "nonesuch",
				"bucket":                      testBucketPrefix,
				"key":                         "state",
				"skip_credentials_validation": true,
			},
			expectedDiags: tfdiags.Diagnostics{
				tfdiags.AttributeValue(
					tfdiags.Error,
					"Invalid region value",
					`Invalid AWS Region: nonesuch`,
					cty.Path{cty.GetAttrStep{Name: "region"}},
				),
			},
		},
		"skip region validation": {
			config: map[string]interface{}{
				"region":                      "nonesuch",
				"bucket":                      testBucketPrefix,
				"key":                         "state",
				"skip_region_validation":      true,
				"skip_credentials_validation": true,
			},
			expectedDiags: nil,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			b := New(encryption.StateEncryptionDisabled())
			configSchema := populateSchema(t, b.ConfigSchema(), hcl2shim.HCL2ValueFromConfigValue(tc.config))

			configSchema, diags := b.PrepareConfig(configSchema)
			if len(diags) > 0 {
				t.Fatal(diags.ErrWithWarnings())
			}

			confDiags := b.Configure(configSchema)
			diags = diags.Append(confDiags)

			if diff := cmp.Diff(diags, tc.expectedDiags, cmp.Comparer(diagnosticComparer)); diff != "" {
				t.Errorf("unexpected diagnostics difference: %s", diff)
			}
		})
	}
}

func TestBackendConfig_RegionEnvVar(t *testing.T) {
	testACC(t)
	config := map[string]interface{}{
		"bucket": testBucketPrefix,
		"key":    "state",
	}

	cases := map[string]struct {
		vars map[string]string
	}{
		"AWS_REGION": {
			vars: map[string]string{
				"AWS_REGION": "us-west-1",
			},
		},

		"AWS_DEFAULT_REGION": {
			vars: map[string]string{
				"AWS_DEFAULT_REGION": "us-west-1",
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			for k, v := range tc.vars {
				t.Setenv(k, v)
			}

			b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(config)).(*Backend)

			if b.awsConfig.Region != "us-west-1" {
				t.Fatalf("Incorrect region was populated")
			}
		})
	}
}

func TestBackendConfig_DynamoDBEndpoint(t *testing.T) {
	testACC(t)

	cases := map[string]struct {
		config   map[string]any
		vars     map[string]string
		expected string
	}{
		"none": {
			expected: "",
		},
		"config": {
			config: map[string]any{
				"dynamodb_endpoint": "dynamo.test",
			},
			expected: "dynamo.test",
		},
		"envvar": {
			vars: map[string]string{
				"AWS_DYNAMODB_ENDPOINT": "dynamo.test",
			},
			expected: "dynamo.test",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			config := map[string]interface{}{
				"region": "us-west-1",
				"bucket": testBucketPrefix,
				"key":    "state",
			}

			if tc.vars != nil {
				for k, v := range tc.vars {
					t.Setenv(k, v)
				}
			}

			if tc.config != nil {
				for k, v := range tc.config {
					config[k] = v
				}
			}

			backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(config))
		})
	}
}

func TestBackendConfig_S3Endpoint(t *testing.T) {
	testACC(t)

	cases := map[string]struct {
		config   map[string]any
		vars     map[string]string
		expected string
	}{
		"none": {
			expected: "",
		},
		"config": {
			config: map[string]any{
				"endpoint": "s3.test",
			},
			expected: "s3.test",
		},
		"envvar": {
			vars: map[string]string{
				"AWS_S3_ENDPOINT": "s3.test",
			},
			expected: "s3.test",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			config := map[string]interface{}{
				"region": "us-west-1",
				"bucket": testBucketPrefix,
				"key":    "state",
			}

			if tc.vars != nil {
				for k, v := range tc.vars {
					t.Setenv(k, v)
				}
			}

			if tc.config != nil {
				for k, v := range tc.config {
					config[k] = v
				}
			}

			backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(config))
		})
	}
}

func TestBackendConfig_STSEndpoint(t *testing.T) {
	testACC(t)

	stsMocks := []*servicemocks.MockEndpoint{
		{
			Request: &servicemocks.MockRequest{Method: "POST", Uri: "/", Body: url.Values{
				"Action":          []string{"AssumeRole"},
				"DurationSeconds": []string{"900"},
				"RoleArn":         []string{servicemocks.MockStsAssumeRoleArn},
				"RoleSessionName": []string{servicemocks.MockStsAssumeRoleSessionName},
				"Version":         []string{"2011-06-15"},
			}.Encode()},
			Response: &servicemocks.MockResponse{StatusCode: 200, Body: servicemocks.MockStsAssumeRoleValidResponseBody, ContentType: "text/xml"},
		},
		{
			Request:  &servicemocks.MockRequest{Method: "POST", Uri: "/", Body: mockStsGetCallerIdentityRequestBody},
			Response: &servicemocks.MockResponse{StatusCode: 200, Body: servicemocks.MockStsGetCallerIdentityValidResponseBody, ContentType: "text/xml"},
		},
	}

	cases := map[string]struct {
		setConfig     bool
		setEnvVars    bool
		expectedDiags tfdiags.Diagnostics
	}{
		"none": {
			expectedDiags: tfdiags.Diagnostics{
				tfdiags.Sourceless(
					tfdiags.Error,
					"Cannot assume IAM Role",
					``,
				),
			},
		},
		"config": {
			setConfig: true,
		},
		"envvar": {
			setEnvVars: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			config := map[string]interface{}{
				"region": "us-west-1",
				"bucket": testBucketPrefix,
				"key":    "state",
				"assume_role": map[string]interface{}{
					"role_arn":     servicemocks.MockStsAssumeRoleArn,
					"session_name": servicemocks.MockStsAssumeRoleSessionName,
				},
			}

			closeSts, _, endpoint := mockdata.GetMockedAwsApiSession("STS", stsMocks)
			defer closeSts()

			if tc.setEnvVars {
				t.Setenv("AWS_STS_ENDPOINT", endpoint)
			}

			if tc.setConfig {
				config["sts_endpoint"] = endpoint
			}

			b := New(encryption.StateEncryptionDisabled())
			configSchema := populateSchema(t, b.ConfigSchema(), hcl2shim.HCL2ValueFromConfigValue(config))

			configSchema, diags := b.PrepareConfig(configSchema)
			if len(diags) > 0 {
				t.Fatal(diags.ErrWithWarnings())
			}

			confDiags := b.Configure(configSchema)
			diags = diags.Append(confDiags)

			if diff := cmp.Diff(diags, tc.expectedDiags, cmp.Comparer(diagnosticSummaryComparer)); diff != "" {
				t.Errorf("unexpected diagnostics difference: %s", diff)
			}
		})
	}
}

func TestBackendConfig_AssumeRole(t *testing.T) {
	testACC(t)

	testCases := []struct {
		Config           map[string]interface{}
		Description      string
		MockStsEndpoints []*servicemocks.MockEndpoint
	}{
		{
			Config: map[string]interface{}{
				"bucket":       testBucketPrefix,
				"key":          "state",
				"region":       "us-west-1",
				"role_arn":     servicemocks.MockStsAssumeRoleArn,
				"session_name": servicemocks.MockStsAssumeRoleSessionName,
			},
			Description: "role_arn",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				{
					Request: &servicemocks.MockRequest{Method: "POST", Uri: "/", Body: url.Values{
						"Action":          []string{"AssumeRole"},
						"DurationSeconds": []string{"900"},
						"RoleArn":         []string{servicemocks.MockStsAssumeRoleArn},
						"RoleSessionName": []string{servicemocks.MockStsAssumeRoleSessionName},
						"Version":         []string{"2011-06-15"},
					}.Encode()},
					Response: &servicemocks.MockResponse{StatusCode: 200, Body: servicemocks.MockStsAssumeRoleValidResponseBody, ContentType: "text/xml"},
				},
				{
					Request:  &servicemocks.MockRequest{Method: "POST", Uri: "/", Body: mockStsGetCallerIdentityRequestBody},
					Response: &servicemocks.MockResponse{StatusCode: 200, Body: servicemocks.MockStsGetCallerIdentityValidResponseBody, ContentType: "text/xml"},
				},
			},
		},
		{
			Config: map[string]interface{}{
				"assume_role_duration_seconds": 3600,
				"bucket":                       testBucketPrefix,
				"key":                          "state",
				"region":                       "us-west-1",
				"role_arn":                     servicemocks.MockStsAssumeRoleArn,
				"session_name":                 servicemocks.MockStsAssumeRoleSessionName,
			},
			Description: "assume_role_duration_seconds",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				{
					Request: &servicemocks.MockRequest{Method: "POST", Uri: "/", Body: url.Values{
						"Action":          []string{"AssumeRole"},
						"DurationSeconds": []string{"3600"},
						"RoleArn":         []string{servicemocks.MockStsAssumeRoleArn},
						"RoleSessionName": []string{servicemocks.MockStsAssumeRoleSessionName},
						"Version":         []string{"2011-06-15"},
					}.Encode()},
					Response: &servicemocks.MockResponse{StatusCode: 200, Body: servicemocks.MockStsAssumeRoleValidResponseBody, ContentType: "text/xml"},
				},
				{
					Request:  &servicemocks.MockRequest{Method: "POST", Uri: "/", Body: mockStsGetCallerIdentityRequestBody},
					Response: &servicemocks.MockResponse{StatusCode: 200, Body: servicemocks.MockStsGetCallerIdentityValidResponseBody, ContentType: "text/xml"},
				},
			},
		},
		{
			Config: map[string]interface{}{
				"bucket":       testBucketPrefix,
				"external_id":  servicemocks.MockStsAssumeRoleExternalId,
				"key":          "state",
				"region":       "us-west-1",
				"role_arn":     servicemocks.MockStsAssumeRoleArn,
				"session_name": servicemocks.MockStsAssumeRoleSessionName,
			},
			Description: "external_id",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				{
					Request: &servicemocks.MockRequest{Method: "POST", Uri: "/", Body: url.Values{
						"Action":          []string{"AssumeRole"},
						"DurationSeconds": []string{"900"},
						"ExternalId":      []string{servicemocks.MockStsAssumeRoleExternalId},
						"RoleArn":         []string{servicemocks.MockStsAssumeRoleArn},
						"RoleSessionName": []string{servicemocks.MockStsAssumeRoleSessionName},
						"Version":         []string{"2011-06-15"},
					}.Encode()},
					Response: &servicemocks.MockResponse{StatusCode: 200, Body: servicemocks.MockStsAssumeRoleValidResponseBody, ContentType: "text/xml"},
				},
				{
					Request:  &servicemocks.MockRequest{Method: "POST", Uri: "/", Body: mockStsGetCallerIdentityRequestBody},
					Response: &servicemocks.MockResponse{StatusCode: 200, Body: servicemocks.MockStsGetCallerIdentityValidResponseBody, ContentType: "text/xml"},
				},
			},
		},
		{
			Config: map[string]interface{}{
				"assume_role_policy": servicemocks.MockStsAssumeRolePolicy,
				"bucket":             "tofu-test",
				"key":                "state",
				"region":             "us-west-1",
				"role_arn":           servicemocks.MockStsAssumeRoleArn,
				"session_name":       servicemocks.MockStsAssumeRoleSessionName,
			},
			Description: "assume_role_policy",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				{
					Request: &servicemocks.MockRequest{Method: "POST", Uri: "/", Body: url.Values{
						"Action":          []string{"AssumeRole"},
						"DurationSeconds": []string{"900"},
						"Policy":          []string{servicemocks.MockStsAssumeRolePolicy},
						"RoleArn":         []string{servicemocks.MockStsAssumeRoleArn},
						"RoleSessionName": []string{servicemocks.MockStsAssumeRoleSessionName},
						"Version":         []string{"2011-06-15"},
					}.Encode()},
					Response: &servicemocks.MockResponse{StatusCode: 200, Body: servicemocks.MockStsAssumeRoleValidResponseBody, ContentType: "text/xml"},
				},
				{
					Request:  &servicemocks.MockRequest{Method: "POST", Uri: "/", Body: mockStsGetCallerIdentityRequestBody},
					Response: &servicemocks.MockResponse{StatusCode: 200, Body: servicemocks.MockStsGetCallerIdentityValidResponseBody, ContentType: "text/xml"},
				},
			},
		},
		{
			Config: map[string]interface{}{
				"assume_role_policy_arns": []interface{}{servicemocks.MockStsAssumeRolePolicyArn},
				"bucket":                  "tofu-test",
				"key":                     "state",
				"region":                  "us-west-1",
				"role_arn":                servicemocks.MockStsAssumeRoleArn,
				"session_name":            servicemocks.MockStsAssumeRoleSessionName,
			},
			Description: "assume_role_policy_arns",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				{
					Request: &servicemocks.MockRequest{Method: "POST", Uri: "/", Body: url.Values{
						"Action":                  []string{"AssumeRole"},
						"DurationSeconds":         []string{"900"},
						"PolicyArns.member.1.arn": []string{servicemocks.MockStsAssumeRolePolicyArn},
						"RoleArn":                 []string{servicemocks.MockStsAssumeRoleArn},
						"RoleSessionName":         []string{servicemocks.MockStsAssumeRoleSessionName},
						"Version":                 []string{"2011-06-15"},
					}.Encode()},
					Response: &servicemocks.MockResponse{StatusCode: 200, Body: servicemocks.MockStsAssumeRoleValidResponseBody, ContentType: "text/xml"},
				},
				{
					Request:  &servicemocks.MockRequest{Method: "POST", Uri: "/", Body: mockStsGetCallerIdentityRequestBody},
					Response: &servicemocks.MockResponse{StatusCode: 200, Body: servicemocks.MockStsGetCallerIdentityValidResponseBody, ContentType: "text/xml"},
				},
			},
		},
		{
			Config: map[string]interface{}{
				"assume_role_tags": map[string]interface{}{
					servicemocks.MockStsAssumeRoleTagKey: servicemocks.MockStsAssumeRoleTagValue,
				},
				"bucket":       "tofu-test",
				"key":          "state",
				"region":       "us-west-1",
				"role_arn":     servicemocks.MockStsAssumeRoleArn,
				"session_name": servicemocks.MockStsAssumeRoleSessionName,
			},
			Description: "assume_role_tags",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				{
					Request: &servicemocks.MockRequest{Method: "POST", Uri: "/", Body: url.Values{
						"Action":              []string{"AssumeRole"},
						"DurationSeconds":     []string{"900"},
						"RoleArn":             []string{servicemocks.MockStsAssumeRoleArn},
						"RoleSessionName":     []string{servicemocks.MockStsAssumeRoleSessionName},
						"Tags.member.1.Key":   []string{servicemocks.MockStsAssumeRoleTagKey},
						"Tags.member.1.Value": []string{servicemocks.MockStsAssumeRoleTagValue},
						"Version":             []string{"2011-06-15"},
					}.Encode()},
					Response: &servicemocks.MockResponse{StatusCode: 200, Body: servicemocks.MockStsAssumeRoleValidResponseBody, ContentType: "text/xml"},
				},
				{
					Request:  &servicemocks.MockRequest{Method: "POST", Uri: "/", Body: mockStsGetCallerIdentityRequestBody},
					Response: &servicemocks.MockResponse{StatusCode: 200, Body: servicemocks.MockStsGetCallerIdentityValidResponseBody, ContentType: "text/xml"},
				},
			},
		},
		{
			Config: map[string]interface{}{
				"assume_role_tags": map[string]interface{}{
					servicemocks.MockStsAssumeRoleTagKey: servicemocks.MockStsAssumeRoleTagValue,
				},
				"assume_role_transitive_tag_keys": []interface{}{servicemocks.MockStsAssumeRoleTagKey},
				"bucket":                          "tofu-test",
				"key":                             "state",
				"region":                          "us-west-1",
				"role_arn":                        servicemocks.MockStsAssumeRoleArn,
				"session_name":                    servicemocks.MockStsAssumeRoleSessionName,
			},
			Description: "assume_role_transitive_tag_keys",
			MockStsEndpoints: []*servicemocks.MockEndpoint{
				{
					Request: &servicemocks.MockRequest{Method: "POST", Uri: "/", Body: url.Values{
						"Action":                     []string{"AssumeRole"},
						"DurationSeconds":            []string{"900"},
						"RoleArn":                    []string{servicemocks.MockStsAssumeRoleArn},
						"RoleSessionName":            []string{servicemocks.MockStsAssumeRoleSessionName},
						"Tags.member.1.Key":          []string{servicemocks.MockStsAssumeRoleTagKey},
						"Tags.member.1.Value":        []string{servicemocks.MockStsAssumeRoleTagValue},
						"TransitiveTagKeys.member.1": []string{servicemocks.MockStsAssumeRoleTagKey},
						"Version":                    []string{"2011-06-15"},
					}.Encode()},
					Response: &servicemocks.MockResponse{StatusCode: 200, Body: servicemocks.MockStsAssumeRoleValidResponseBody, ContentType: "text/xml"},
				},
				{
					Request:  &servicemocks.MockRequest{Method: "POST", Uri: "/", Body: mockStsGetCallerIdentityRequestBody},
					Response: &servicemocks.MockResponse{StatusCode: 200, Body: servicemocks.MockStsGetCallerIdentityValidResponseBody, ContentType: "text/xml"},
				},
			},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.Description, func(t *testing.T) {
			closeSts, _, endpoint := mockdata.GetMockedAwsApiSession("STS", testCase.MockStsEndpoints)
			defer closeSts()

			testCase.Config["sts_endpoint"] = endpoint

			b := New(encryption.StateEncryptionDisabled())
			diags := b.Configure(populateSchema(t, b.ConfigSchema(), hcl2shim.HCL2ValueFromConfigValue(testCase.Config)))

			if diags.HasErrors() {
				for _, diag := range diags {
					t.Errorf("unexpected error: %s", diag.Description().Summary)
				}
			}
		})
	}
}

func TestBackendConfig_PrepareConfigValidation(t *testing.T) {
	cases := map[string]struct {
		config      cty.Value
		expectedErr string
	}{
		"null bucket": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket": cty.NullVal(cty.String),
				"key":    cty.StringVal("test"),
				"region": cty.StringVal("us-west-2"),
			}),
			expectedErr: `The "bucket" attribute value must not be empty.`,
		},
		"empty bucket": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket": cty.StringVal(""),
				"key":    cty.StringVal("test"),
				"region": cty.StringVal("us-west-2"),
			}),
			expectedErr: `The "bucket" attribute value must not be empty.`,
		},
		"null key": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket": cty.StringVal("test"),
				"key":    cty.NullVal(cty.String),
				"region": cty.StringVal("us-west-2"),
			}),
			expectedErr: `The "key" attribute value must not be empty.`,
		},
		"empty key": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket": cty.StringVal("test"),
				"key":    cty.StringVal(""),
				"region": cty.StringVal("us-west-2"),
			}),
			expectedErr: `The "key" attribute value must not be empty.`,
		},
		"key with leading slash": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket": cty.StringVal("test"),
				"key":    cty.StringVal("/leading-slash"),
				"region": cty.StringVal("us-west-2"),
			}),
			expectedErr: `The "key" attribute value must not start or end with with "/".`,
		},
		"key with trailing slash": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket": cty.StringVal("test"),
				"key":    cty.StringVal("trailing-slash/"),
				"region": cty.StringVal("us-west-2"),
			}),
			expectedErr: `The "key" attribute value must not start or end with with "/".`,
		},
		"null region": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket": cty.StringVal("test"),
				"key":    cty.StringVal("test"),
				"region": cty.NullVal(cty.String),
			}),
			expectedErr: `The "region" attribute or the "AWS_REGION" or "AWS_DEFAULT_REGION" environment variables must be set.`,
		},
		"empty region": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket": cty.StringVal("test"),
				"key":    cty.StringVal("test"),
				"region": cty.StringVal(""),
			}),
			expectedErr: `The "region" attribute or the "AWS_REGION" or "AWS_DEFAULT_REGION" environment variables must be set.`,
		},
		"workspace_key_prefix with leading slash": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket":               cty.StringVal("test"),
				"key":                  cty.StringVal("test"),
				"region":               cty.StringVal("us-west-2"),
				"workspace_key_prefix": cty.StringVal("/env"),
			}),
			expectedErr: `The "workspace_key_prefix" attribute value must not start with "/".`,
		},
		"workspace_key_prefix with trailing slash": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket":               cty.StringVal("test"),
				"key":                  cty.StringVal("test"),
				"region":               cty.StringVal("us-west-2"),
				"workspace_key_prefix": cty.StringVal("env/"),
			}),
			expectedErr: `The "workspace_key_prefix" attribute value must not start with "/".`,
		},
		"encryption key conflict": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket":               cty.StringVal("test"),
				"key":                  cty.StringVal("test"),
				"region":               cty.StringVal("us-west-2"),
				"workspace_key_prefix": cty.StringVal("env"),
				"sse_customer_key":     cty.StringVal("1hwbcNPGWL+AwDiyGmRidTWAEVmCWMKbEHA+Es8w75o="),
				"kms_key_id":           cty.StringVal("arn:aws:kms:us-west-2:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab"),
			}),
			expectedErr: `Only one of "kms_key_id" and "sse_customer_key" can be set`,
		},
		"allowed forbidden account ids conflict": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket":                cty.StringVal("test"),
				"key":                   cty.StringVal("test"),
				"region":                cty.StringVal("us-west-2"),
				"allowed_account_ids":   cty.SetVal([]cty.Value{cty.StringVal("111111111111")}),
				"forbidden_account_ids": cty.SetVal([]cty.Value{cty.StringVal("111111111111")}),
			}),
			expectedErr: "Invalid Attribute Combination: Only one of allowed_account_ids, forbidden_account_ids can be set.",
		},
		"invalid retry mode": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket":     cty.StringVal("test"),
				"key":        cty.StringVal("test"),
				"region":     cty.StringVal("us-west-2"),
				"retry_mode": cty.StringVal("xyz"),
			}),
			expectedErr: `Invalid retry mode: Valid values are "standard" and "adaptive".`,
		},
		"s3 endpoint conflict": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket":   cty.StringVal("test"),
				"key":      cty.StringVal("test"),
				"region":   cty.StringVal("us-west-2"),
				"endpoint": cty.StringVal("x1"),
				"endpoints": cty.ObjectVal(map[string]cty.Value{
					"s3": cty.StringVal("x2"),
				}),
			}),
			expectedErr: `Invalid Attribute Combination: Only one of endpoints.s3, endpoint can be set.`,
		},
		"iam endpoint conflict": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket":       cty.StringVal("test"),
				"key":          cty.StringVal("test"),
				"region":       cty.StringVal("us-west-2"),
				"iam_endpoint": cty.StringVal("x1"),
				"endpoints": cty.ObjectVal(map[string]cty.Value{
					"iam": cty.StringVal("x2"),
				}),
			}),
			expectedErr: `Invalid Attribute Combination: Only one of endpoints.iam, iam_endpoint can be set.`,
		},
		"sts endpoint conflict": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket":       cty.StringVal("test"),
				"key":          cty.StringVal("test"),
				"region":       cty.StringVal("us-west-2"),
				"sts_endpoint": cty.StringVal("x1"),
				"endpoints": cty.ObjectVal(map[string]cty.Value{
					"sts": cty.StringVal("x2"),
				}),
			}),
			expectedErr: `Invalid Attribute Combination: Only one of endpoints.sts, sts_endpoint can be set.`,
		},
		"dynamodb endpoint conflict": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket":            cty.StringVal("test"),
				"key":               cty.StringVal("test"),
				"region":            cty.StringVal("us-west-2"),
				"dynamodb_endpoint": cty.StringVal("x1"),
				"endpoints": cty.ObjectVal(map[string]cty.Value{
					"dynamodb": cty.StringVal("x2"),
				}),
			}),
			expectedErr: `Invalid Attribute Combination: Only one of endpoints.dynamodb, dynamodb_endpoint can be set.`,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			servicemocks.StashEnv(t)

			b := New(encryption.StateEncryptionDisabled())

			_, valDiags := b.PrepareConfig(populateSchema(t, b.ConfigSchema(), tc.config))
			if tc.expectedErr != "" {
				if valDiags.Err() != nil {
					actualErr := valDiags.Err().Error()
					if !strings.Contains(actualErr, tc.expectedErr) {
						t.Fatalf("unexpected validation result: %v", valDiags.Err())
					}
				} else {
					t.Fatal("expected an error, got none")
				}
			} else if valDiags.Err() != nil {
				t.Fatalf("expected no error, got %s", valDiags.Err())
			}
		})
	}
}

func TestBackendConfig_PrepareConfigValidationWarnings(t *testing.T) {
	cases := map[string]struct {
		config       cty.Value
		expectedWarn string
	}{
		"deprecated force path style": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket":           cty.StringVal("test"),
				"key":              cty.StringVal("test"),
				"region":           cty.StringVal("us-west-2"),
				"force_path_style": cty.BoolVal(false),
			}),
			expectedWarn: `Deprecated Parameter: Parameter "force_path_style" is deprecated. Use "use_path_style" instead.`,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			servicemocks.StashEnv(t)

			b := New(encryption.StateEncryptionDisabled())

			_, diags := b.PrepareConfig(populateSchema(t, b.ConfigSchema(), tc.config))
			if tc.expectedWarn != "" {
				if err := diags.ErrWithWarnings(); err != nil {
					if !strings.Contains(err.Error(), tc.expectedWarn) {
						t.Fatalf("unexpected validation result: %v", err)
					}
				} else {
					t.Fatal("expected a warning, got none")
				}
			} else if err := diags.ErrWithWarnings(); err != nil {
				t.Fatalf("expected no warnings, got %s", err)
			}
		})
	}
}

func TestBackendConfig_PrepareConfigWithEnvVars(t *testing.T) {
	cases := map[string]struct {
		config      cty.Value
		vars        map[string]string
		expectedErr string
	}{
		"region env var AWS_REGION": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket": cty.StringVal("test"),
				"key":    cty.StringVal("test"),
				"region": cty.NullVal(cty.String),
			}),
			vars: map[string]string{
				"AWS_REGION": "us-west-1",
			},
		},
		"region env var AWS_DEFAULT_REGION": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket": cty.StringVal("test"),
				"key":    cty.StringVal("test"),
				"region": cty.NullVal(cty.String),
			}),
			vars: map[string]string{
				"AWS_DEFAULT_REGION": "us-west-1",
			},
		},
		"encryption key conflict": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket":               cty.StringVal("test"),
				"key":                  cty.StringVal("test"),
				"region":               cty.StringVal("us-west-2"),
				"workspace_key_prefix": cty.StringVal("env"),
				"kms_key_id":           cty.StringVal("arn:aws:kms:us-west-2:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab"),
			}),
			vars: map[string]string{
				"AWS_SSE_CUSTOMER_KEY": "1hwbcNPGWL+AwDiyGmRidTWAEVmCWMKbEHA+Es8w75o=",
			},
			expectedErr: `Only one of "kms_key_id" and the environment variable "AWS_SSE_CUSTOMER_KEY" can be set`,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			servicemocks.StashEnv(t)

			b := New(encryption.StateEncryptionDisabled())

			for k, v := range tc.vars {
				t.Setenv(k, v)
			}

			_, valDiags := b.PrepareConfig(populateSchema(t, b.ConfigSchema(), tc.config))
			if tc.expectedErr != "" {
				if valDiags.Err() != nil {
					actualErr := valDiags.Err().Error()
					if !strings.Contains(actualErr, tc.expectedErr) {
						t.Fatalf("unexpected validation result: %v", valDiags.Err())
					}
				} else {
					t.Fatal("expected an error, got none")
				}
			} else if valDiags.Err() != nil {
				t.Fatalf("expected no error, got %s", valDiags.Err())
			}
		})
	}
}

// TestBackendConfig_proxy tests proxy configuration
func TestBackendConfig_proxy(t *testing.T) {
	testACC(t)

	newURL := func(rawURL string) *url.URL {
		o, err := url.Parse(rawURL)
		if err != nil {
			panic(err)
		}
		return o
	}

	cases := map[string]struct {
		config       cty.Value
		calledURL    string
		envVars      map[string]string
		wantProxyURL *url.URL

		// wantErrSubstr contains the part indicating proxy address
		wantErrSubstr string
	}{
		"shall set proxy using http_proxy config attr": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket":     cty.StringVal("test"),
				"key":        cty.StringVal("test"),
				"http_proxy": cty.StringVal("http://foo.bar"),
			}),
			calledURL:    "http://qux.quxx",
			wantProxyURL: newURL("http://foo.bar"),
		},
		"shall set proxy using HTTP_PROXY envvar": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket": cty.StringVal("test"),
				"key":    cty.StringVal("test"),
			}),
			envVars: map[string]string{
				"HTTP_PROXY": "http://foo.com",
			},
			calledURL:    "http://qux.quxx",
			wantProxyURL: newURL("http://foo.com"),
		},
		"shall set proxy using http_proxy config attr when HTTP_PROXY envvar is also set": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket":     cty.StringVal("test"),
				"key":        cty.StringVal("test"),
				"http_proxy": cty.StringVal("http://foo.bar"),
			}),
			envVars: map[string]string{
				"HTTP_PROXY": "http://foo.com",
			},
			calledURL:    "http://qux.quxx",
			wantProxyURL: newURL("http://foo.bar"),
		},
		"shall set proxy using https_proxy config attr": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket":      cty.StringVal("test"),
				"key":         cty.StringVal("test"),
				"https_proxy": cty.StringVal("https://foo.bar"),
			}),
			calledURL:     "https://qux.quxx",
			wantErrSubstr: "proxyconnect tcp: dial tcp: lookup foo.bar",
		},
		"shall set proxy using HTTPS_PROXY envvar": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket": cty.StringVal("test"),
				"key":    cty.StringVal("test"),
			}),
			envVars: map[string]string{
				"HTTPS_PROXY": "https://foo.baz",
			},
			calledURL:     "https://qux.quxx",
			wantErrSubstr: "proxyconnect tcp: dial tcp: lookup foo.baz",
		},
		"shall set proxy using https_proxy config attr when HTTPS_PROXY envvar is also set": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket":      cty.StringVal("test"),
				"key":         cty.StringVal("test"),
				"https_proxy": cty.StringVal("https://foo.bar"),
			}),
			envVars: map[string]string{
				"HTTPS_PROXY": "https://foo.com",
			},
			calledURL:     "https://qux.quxx",
			wantErrSubstr: "proxyconnect tcp: dial tcp: lookup foo.bar",
		},
		"shall satisfy no_proxy config attr": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket":   cty.StringVal("test"),
				"key":      cty.StringVal("test"),
				"no_proxy": cty.StringVal("http://foo.bar,1.2.3.4"),
			}),
			calledURL: "http://foo.bar",
		},
		"shall satisfy no proxy set using NO_PROXY envvar": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket": cty.StringVal("test"),
				"key":    cty.StringVal("test"),
			}),
			envVars: map[string]string{
				"NO_PROXY": "http://foo.bar,1.2.3.4",
			},
			calledURL: "http://foo.bar",
		},
		"shall satisfy no_proxy config attr when envvar NO_PROXY is also set": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket":   cty.StringVal("test"),
				"key":      cty.StringVal("test"),
				"no_proxy": cty.StringVal("http://foo.qux,1.2.3.4"),
			}),
			envVars: map[string]string{
				"NO_PROXY": "http://foo.bar",
			},
			calledURL: "http://foo.qux",
		},
		"shall satisfy use http_proxy when no_proxy is also set to identical value": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket":     cty.StringVal("test"),
				"key":        cty.StringVal("test"),
				"http_proxy": cty.StringVal("http://foo.bar"),
				"no_proxy":   cty.StringVal("http://foo.bar"),
			}),
			calledURL:    "http://qux.quxx",
			wantProxyURL: newURL("http://foo.bar"),
		},
		"shall satisfy use https_proxy when no_proxy is also set to identical value": {
			config: cty.ObjectVal(map[string]cty.Value{
				"bucket":      cty.StringVal("test"),
				"key":         cty.StringVal("test"),
				"https_proxy": cty.StringVal("https://foo.bar"),
				"no_proxy":    cty.StringVal("http://foo.bar"),
			}),
			calledURL:     "https://qux.quxx",
			wantErrSubstr: "proxyconnect tcp: dial tcp: lookup foo.bar",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			for k, v := range tc.envVars {
				t.Setenv(k, v)
			}

			b := New(encryption.StateEncryptionDisabled())

			got := b.Configure(populateSchema(t, b.ConfigSchema(), tc.config))
			if got.HasErrors() != (tc.wantErrSubstr != "") {
				t.Fatalf("unexpected error: %v", got.Err())
			}

			switch got.HasErrors() {
			case true:
				if !strings.Contains(got.Err().Error(), tc.wantErrSubstr) {
					t.Fatalf("unexpected error: want= %s, got= %s", tc.wantErrSubstr, got.Err().Error())
				}
			case false:
				gotProxyURL, err := b.(*Backend).awsConfig.HTTPClient.(*awshttp.BuildableClient).GetTransport().Proxy(&http.Request{
					URL: newURL(tc.calledURL),
				})
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}

				if !reflect.DeepEqual(gotProxyURL, tc.wantProxyURL) {
					t.Fatalf("unexpected proxy URL: want= %s, got= %s", tc.wantProxyURL, gotProxyURL)
				}
			}
		})
	}
}

func TestBackend(t *testing.T) {
	testACC(t)

	bucketName := fmt.Sprintf("%s-%x", testBucketPrefix, time.Now().Unix())
	keyName := "testState"

	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"bucket":  bucketName,
		"key":     keyName,
		"encrypt": true,
		"region":  "us-west-1",
	})).(*Backend)

	ctx := context.TODO()
	createS3Bucket(ctx, t, b.s3Client, bucketName, b.awsConfig.Region)
	defer deleteS3Bucket(ctx, t, b.s3Client, bucketName)

	backend.TestBackendStates(t, b)
}

func TestBackendLocked(t *testing.T) {
	testACC(t)

	bucketName := fmt.Sprintf("%s-%x", testBucketPrefix, time.Now().Unix())
	keyName := "test/state"

	b1 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"bucket":         bucketName,
		"key":            keyName,
		"encrypt":        true,
		"dynamodb_table": bucketName,
		"region":         "us-west-1",
	})).(*Backend)

	b2 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"bucket":         bucketName,
		"key":            keyName,
		"encrypt":        true,
		"dynamodb_table": bucketName,
		"region":         "us-west-1",
	})).(*Backend)

	ctx := context.TODO()
	createS3Bucket(ctx, t, b1.s3Client, bucketName, b1.awsConfig.Region)
	defer deleteS3Bucket(ctx, t, b1.s3Client, bucketName)
	createDynamoDBTable(ctx, t, b1.dynClient, bucketName)
	defer deleteDynamoDBTable(ctx, t, b1.dynClient, bucketName)

	backend.TestBackendStateLocks(t, b1, b2)
	backend.TestBackendStateForceUnlock(t, b1, b2)
}

func TestBackendSSECustomerKeyConfig(t *testing.T) {
	testACC(t)

	testCases := map[string]struct {
		customerKey string
		expectedErr string
	}{
		"invalid length": {
			customerKey: "test",
			expectedErr: `sse_customer_key must be 44 characters in length`,
		},
		"invalid encoding": {
			customerKey: "====CT70aTYB2JGff7AjQtwbiLkwH4npICay1PWtmdka",
			expectedErr: `sse_customer_key must be base64 encoded`,
		},
		"valid": {
			customerKey: "4Dm1n4rphuFgawxuzY/bEfvLf6rYK0gIjfaDSLlfXNk=",
		},
	}

	for name, testCase := range testCases {
		testCase := testCase

		t.Run(name, func(t *testing.T) {
			bucketName := fmt.Sprintf("%s-%x", testBucketPrefix, time.Now().Unix())
			config := map[string]interface{}{
				"bucket":           bucketName,
				"encrypt":          true,
				"key":              "test-SSE-C",
				"sse_customer_key": testCase.customerKey,
				"region":           "us-west-1",
			}

			b := New(encryption.StateEncryptionDisabled()).(*Backend)
			diags := b.Configure(populateSchema(t, b.ConfigSchema(), hcl2shim.HCL2ValueFromConfigValue(config)))

			if testCase.expectedErr != "" {
				if diags.Err() != nil {
					actualErr := diags.Err().Error()
					if !strings.Contains(actualErr, testCase.expectedErr) {
						t.Fatalf("unexpected validation result: %v", diags.Err())
					}
				} else {
					t.Fatal("expected an error, got none")
				}
			} else {
				if diags.Err() != nil {
					t.Fatalf("expected no error, got %s", diags.Err())
				}
				if string(b.customerEncryptionKey) != string(must(base64.StdEncoding.DecodeString(testCase.customerKey))) {
					t.Fatal("unexpected value for customer encryption key")
				}

				ctx := context.TODO()
				createS3Bucket(ctx, t, b.s3Client, bucketName, b.awsConfig.Region)
				defer deleteS3Bucket(ctx, t, b.s3Client, bucketName)

				backend.TestBackendStates(t, b)
			}
		})
	}
}

func TestBackendSSECustomerKeyEnvVar(t *testing.T) {
	testACC(t)

	testCases := map[string]struct {
		customerKey string
		expectedErr string
	}{
		"invalid length": {
			customerKey: "test",
			expectedErr: `The environment variable "AWS_SSE_CUSTOMER_KEY" must be 44 characters in length`,
		},
		"invalid encoding": {
			customerKey: "====CT70aTYB2JGff7AjQtwbiLkwH4npICay1PWtmdka",
			expectedErr: `The environment variable "AWS_SSE_CUSTOMER_KEY" must be base64 encoded`,
		},
		"valid": {
			customerKey: "4Dm1n4rphuFgawxuzY/bEfvLf6rYK0gIjfaDSLlfXNk=",
		},
	}

	for name, testCase := range testCases {
		testCase := testCase

		t.Run(name, func(t *testing.T) {
			bucketName := fmt.Sprintf("%s-%x", testBucketPrefix, time.Now().Unix())
			config := map[string]interface{}{
				"bucket":  bucketName,
				"encrypt": true,
				"key":     "test-SSE-C",
				"region":  "us-west-1",
			}

			t.Setenv("AWS_SSE_CUSTOMER_KEY", testCase.customerKey)

			b := New(encryption.StateEncryptionDisabled()).(*Backend)
			diags := b.Configure(populateSchema(t, b.ConfigSchema(), hcl2shim.HCL2ValueFromConfigValue(config)))

			if testCase.expectedErr != "" {
				if diags.Err() != nil {
					actualErr := diags.Err().Error()
					if !strings.Contains(actualErr, testCase.expectedErr) {
						t.Fatalf("unexpected validation result: %v", diags.Err())
					}
				} else {
					t.Fatal("expected an error, got none")
				}
			} else {
				if diags.Err() != nil {
					t.Fatalf("expected no error, got %s", diags.Err())
				}
				if string(b.customerEncryptionKey) != string(must(base64.StdEncoding.DecodeString(testCase.customerKey))) {
					t.Fatal("unexpected value for customer encryption key")
				}

				ctx := context.TODO()
				createS3Bucket(ctx, t, b.s3Client, bucketName, b.awsConfig.Region)
				defer deleteS3Bucket(ctx, t, b.s3Client, bucketName)

				backend.TestBackendStates(t, b)
			}
		})
	}
}

// add some extra junk in S3 to try and confuse the env listing.
func TestBackendExtraPaths(t *testing.T) {
	testACC(t)
	bucketName := fmt.Sprintf("%s-%x", testBucketPrefix, time.Now().Unix())
	keyName := "test/state/tfstate"

	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"bucket":  bucketName,
		"key":     keyName,
		"encrypt": true,
	})).(*Backend)

	ctx := context.TODO()
	createS3Bucket(ctx, t, b.s3Client, bucketName, b.awsConfig.Region)
	defer deleteS3Bucket(ctx, t, b.s3Client, bucketName)

	// put multiple states in old env paths.
	s1 := states.NewState()
	s2 := states.NewState()

	// RemoteClient to Put things in various paths
	client := &RemoteClient{
		s3Client:             b.s3Client,
		dynClient:            b.dynClient,
		bucketName:           b.bucketName,
		path:                 b.path("s1"),
		serverSideEncryption: b.serverSideEncryption,
		acl:                  b.acl,
		kmsKeyID:             b.kmsKeyID,
		ddbTable:             b.ddbTable,
	}

	// Write the first state
	stateMgr := remote.NewState(client, encryption.StateEncryptionDisabled())
	if err := stateMgr.WriteState(s1); err != nil {
		t.Fatal(err)
	}
	if err := stateMgr.PersistState(nil); err != nil {
		t.Fatal(err)
	}

	// Write the second state
	// Note a new state manager - otherwise, because these
	// states are equal, the state will not Put to the remote
	client.path = b.path("s2")
	stateMgr2 := remote.NewState(client, encryption.StateEncryptionDisabled())
	if err := stateMgr2.WriteState(s2); err != nil {
		t.Fatal(err)
	}
	if err := stateMgr2.PersistState(nil); err != nil {
		t.Fatal(err)
	}

	s2Lineage := stateMgr2.StateSnapshotMeta().Lineage

	if err := checkStateList(b, []string{"default", "s1", "s2"}); err != nil {
		t.Fatal(err)
	}

	// put a state in an env directory name
	client.path = b.workspaceKeyPrefix + "/error"
	if err := stateMgr.WriteState(states.NewState()); err != nil {
		t.Fatal(err)
	}
	if err := stateMgr.PersistState(nil); err != nil {
		t.Fatal(err)
	}
	if err := checkStateList(b, []string{"default", "s1", "s2"}); err != nil {
		t.Fatal(err)
	}

	// add state with the wrong key for an existing env
	client.path = b.workspaceKeyPrefix + "/s2/notTestState"
	if err := stateMgr.WriteState(states.NewState()); err != nil {
		t.Fatal(err)
	}
	if err := stateMgr.PersistState(nil); err != nil {
		t.Fatal(err)
	}
	if err := checkStateList(b, []string{"default", "s1", "s2"}); err != nil {
		t.Fatal(err)
	}

	// remove the state with extra subkey
	if err := client.Delete(); err != nil {
		t.Fatal(err)
	}

	// delete the real workspace
	if err := b.DeleteWorkspace("s2", true); err != nil {
		t.Fatal(err)
	}

	if err := checkStateList(b, []string{"default", "s1"}); err != nil {
		t.Fatal(err)
	}

	// fetch that state again, which should produce a new lineage
	s2Mgr, err := b.StateMgr("s2")
	if err != nil {
		t.Fatal(err)
	}
	if err := s2Mgr.RefreshState(); err != nil {
		t.Fatal(err)
	}

	if s2Mgr.(*remote.State).StateSnapshotMeta().Lineage == s2Lineage {
		t.Fatal("state s2 was not deleted")
	}
	_ = s2Mgr.State() // We need the side-effect
	s2Lineage = stateMgr.StateSnapshotMeta().Lineage

	// add a state with a key that matches an existing environment dir name
	client.path = b.workspaceKeyPrefix + "/s2/"
	if err := stateMgr.WriteState(states.NewState()); err != nil {
		t.Fatal(err)
	}
	if err := stateMgr.PersistState(nil); err != nil {
		t.Fatal(err)
	}

	// make sure s2 is OK
	s2Mgr, err = b.StateMgr("s2")
	if err != nil {
		t.Fatal(err)
	}
	if err := s2Mgr.RefreshState(); err != nil {
		t.Fatal(err)
	}

	if stateMgr.StateSnapshotMeta().Lineage != s2Lineage {
		t.Fatal("we got the wrong state for s2")
	}

	if err := checkStateList(b, []string{"default", "s1", "s2"}); err != nil {
		t.Fatal(err)
	}
}

// ensure we can separate the workspace prefix when it also matches the prefix
// of the workspace name itself.
func TestBackendPrefixInWorkspace(t *testing.T) {
	testACC(t)
	bucketName := fmt.Sprintf("%s-%x", testBucketPrefix, time.Now().Unix())

	b := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"bucket":               bucketName,
		"key":                  "test-env.tfstate",
		"workspace_key_prefix": "env",
	})).(*Backend)

	ctx := context.TODO()
	createS3Bucket(ctx, t, b.s3Client, bucketName, b.awsConfig.Region)
	defer deleteS3Bucket(ctx, t, b.s3Client, bucketName)

	// get a state that contains the prefix as a substring
	sMgr, err := b.StateMgr("env-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := sMgr.RefreshState(); err != nil {
		t.Fatal(err)
	}

	if err := checkStateList(b, []string{"default", "env-1"}); err != nil {
		t.Fatal(err)
	}
}

func TestKeyEnv(t *testing.T) {
	testACC(t)
	keyName := "some/paths/tfstate"

	bucket0Name := fmt.Sprintf("%s-%x-0", testBucketPrefix, time.Now().Unix())
	b0 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"bucket":               bucket0Name,
		"key":                  keyName,
		"encrypt":              true,
		"workspace_key_prefix": "",
	})).(*Backend)

	ctx := context.TODO()
	createS3Bucket(ctx, t, b0.s3Client, bucket0Name, b0.awsConfig.Region)
	defer deleteS3Bucket(ctx, t, b0.s3Client, bucket0Name)

	bucket1Name := fmt.Sprintf("%s-%x-1", testBucketPrefix, time.Now().Unix())
	b1 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"bucket":               bucket1Name,
		"key":                  keyName,
		"encrypt":              true,
		"workspace_key_prefix": "project/env:",
	})).(*Backend)

	createS3Bucket(ctx, t, b1.s3Client, bucket1Name, b1.awsConfig.Region)
	defer deleteS3Bucket(ctx, t, b1.s3Client, bucket1Name)

	bucket2Name := fmt.Sprintf("%s-%x-2", testBucketPrefix, time.Now().Unix())
	b2 := backend.TestBackendConfig(t, New(encryption.StateEncryptionDisabled()), backend.TestWrapConfig(map[string]interface{}{
		"bucket":  bucket2Name,
		"key":     keyName,
		"encrypt": true,
	})).(*Backend)

	createS3Bucket(ctx, t, b2.s3Client, bucket2Name, b2.awsConfig.Region)
	defer deleteS3Bucket(ctx, t, b2.s3Client, bucket2Name)

	if err := testGetWorkspaceForKey(b0, "some/paths/tfstate", ""); err != nil {
		t.Fatal(err)
	}

	if err := testGetWorkspaceForKey(b0, "ws1/some/paths/tfstate", "ws1"); err != nil {
		t.Fatal(err)
	}

	if err := testGetWorkspaceForKey(b1, "project/env:/ws1/some/paths/tfstate", "ws1"); err != nil {
		t.Fatal(err)
	}

	if err := testGetWorkspaceForKey(b1, "project/env:/ws2/some/paths/tfstate", "ws2"); err != nil {
		t.Fatal(err)
	}

	if err := testGetWorkspaceForKey(b2, "env:/ws3/some/paths/tfstate", "ws3"); err != nil {
		t.Fatal(err)
	}

	backend.TestBackendStates(t, b0)
	backend.TestBackendStates(t, b1)
	backend.TestBackendStates(t, b2)
}

func Test_pathString(t *testing.T) {
	tests := []struct {
		name     string
		path     cty.Path
		expected string
	}{
		{
			name:     "Simple Path",
			path:     cty.Path{cty.GetAttrStep{Name: "attr"}},
			expected: "attr",
		},
		{
			name: "Nested Path",
			path: cty.Path{
				cty.GetAttrStep{Name: "parent"},
				cty.GetAttrStep{Name: "child"},
			},
			expected: "parent.child",
		},
		{
			name: "Indexed Path",
			path: cty.Path{
				cty.GetAttrStep{Name: "array"},
				cty.IndexStep{Key: cty.NumberIntVal(0)},
			},
			expected: "array[0]",
		},
		{
			name: "Mixed Path",
			path: cty.Path{
				cty.GetAttrStep{Name: "parent"},
				cty.IndexStep{Key: cty.StringVal("key")},
				cty.GetAttrStep{Name: "child"},
			},
			expected: "parent[key].child",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := pathString(test.path)
			if result != test.expected {
				t.Errorf("Expected: %s, Got: %s", test.expected, result)
			}
		})
	}
}

func TestBackend_includeProtoIfNecessary(t *testing.T) {
	tests := []struct {
		name     string
		provided string
		expected string
	}{
		{
			name:     "Unmodified S3",
			provided: "https://s3.us-east-1.amazonaws.com",
			expected: "https://s3.us-east-1.amazonaws.com",
		},
		{
			name:     "Modified S3",
			provided: "s3.us-east-1.amazonaws.com",
			expected: "https://s3.us-east-1.amazonaws.com",
		},
		{
			name:     "Unmodified With Port",
			provided: "http://localhost:9000/",
			expected: "http://localhost:9000/",
		},
		{
			name:     "Modified With Port",
			provided: "localhost:9000/",
			expected: "https://localhost:9000/",
		},
		{
			name:     "Umodified with strange proto",
			provided: "ftp://localhost:9000/",
			expected: "ftp://localhost:9000/",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := includeProtoIfNecessary(test.provided)
			if result != test.expected {
				t.Errorf("Expected: %s, Got: %s", test.expected, result)
			}
		})
	}
}

func TestBackend_schemaCoercionMinimal(t *testing.T) {
	example := cty.ObjectVal(map[string]cty.Value{
		"bucket": cty.StringVal("my-bucket"),
		"key":    cty.StringVal("state.tf"),
	})
	schema := New(encryption.StateEncryptionDisabled()).ConfigSchema()
	_, err := schema.CoerceValue(example)
	if err != nil {
		t.Errorf("Unexpected error: %s", err.Error())
	}
}

func testGetWorkspaceForKey(b *Backend, key string, expected string) error {
	if actual := b.keyEnv(key); actual != expected {
		return fmt.Errorf("incorrect workspace for key[%q]. Expected[%q]: Actual[%q]", key, expected, actual)
	}
	return nil
}

func checkStateList(b backend.Backend, expected []string) error {
	states, err := b.Workspaces()
	if err != nil {
		return err
	}

	if !reflect.DeepEqual(states, expected) {
		return fmt.Errorf("incorrect states listed: %q", states)
	}
	return nil
}

func createS3Bucket(ctx context.Context, t *testing.T, s3Client *s3.Client, bucketName, region string) {
	createBucketReq := &s3.CreateBucketInput{
		Bucket: &bucketName,
	}

	// Regions outside of us-east-1 require the appropriate LocationConstraint
	// to be specified in order to create the bucket in the desired region.
	// https://docs.aws.amazon.com/cli/latest/reference/s3api/create-bucket.html
	if region != "us-east-1" {
		createBucketReq.CreateBucketConfiguration = &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(region),
		}
	}

	// Be clear about what we're doing in case the user needs to clean
	// this up later.
	t.Logf("creating S3 bucket %s in %s", bucketName, region)
	_, err := s3Client.CreateBucket(ctx, createBucketReq)
	if err != nil {
		t.Fatal("failed to create test S3 bucket:", err)
	}
}

func deleteS3Bucket(ctx context.Context, t *testing.T, s3Client *s3.Client, bucketName string) {
	warning := "WARNING: Failed to delete the test S3 bucket. It may have been left in your AWS account and may incur storage charges. (error was %s)"

	// first we have to get rid of the env objects, or we can't delete the bucket
	resp, err := s3Client.ListObjects(ctx, &s3.ListObjectsInput{Bucket: &bucketName})
	if err != nil {
		t.Logf(warning, err)
		return
	}
	for _, obj := range resp.Contents {
		if _, err := s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &bucketName, Key: obj.Key}); err != nil {
			// this will need cleanup no matter what, so just warn and exit
			t.Logf(warning, err)
			return
		}
	}

	if _, err := s3Client.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: &bucketName}); err != nil {
		t.Logf(warning, err)
	}
}

// create the dynamoDB table, and wait until we can query it.
func createDynamoDBTable(ctx context.Context, t *testing.T, dynClient *dynamodb.Client, tableName string) {
	createInput := &dynamodb.CreateTableInput{
		AttributeDefinitions: []dtypes.AttributeDefinition{
			{
				AttributeName: aws.String("LockID"),
				AttributeType: dtypes.ScalarAttributeTypeS,
			},
		},
		KeySchema: []dtypes.KeySchemaElement{
			{
				AttributeName: aws.String("LockID"),
				KeyType:       dtypes.KeyTypeHash,
			},
		},
		ProvisionedThroughput: &dtypes.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(5),
			WriteCapacityUnits: aws.Int64(5),
		},
		TableName: aws.String(tableName),
	}

	_, err := dynClient.CreateTable(ctx, createInput)
	if err != nil {
		t.Fatal(err)
	}

	// now wait until it's ACTIVE
	start := time.Now()
	time.Sleep(time.Second)

	describeInput := &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}

	for {
		resp, err := dynClient.DescribeTable(ctx, describeInput)
		if err != nil {
			t.Fatal(err)
		}

		if resp.Table.TableStatus == dtypes.TableStatusActive {
			return
		}

		if time.Since(start) > time.Minute {
			t.Fatalf("timed out creating DynamoDB table %s", tableName)
		}

		time.Sleep(3 * time.Second)
	}

}

func deleteDynamoDBTable(ctx context.Context, t *testing.T, dynClient *dynamodb.Client, tableName string) {
	params := &dynamodb.DeleteTableInput{
		TableName: aws.String(tableName),
	}
	_, err := dynClient.DeleteTable(ctx, params)
	if err != nil {
		t.Logf("WARNING: Failed to delete the test DynamoDB table %q. It has been left in your AWS account and may incur charges. (error was %s)", tableName, err)
	}
}

func populateSchema(t *testing.T, schema *configschema.Block, value cty.Value) cty.Value {
	ty := schema.ImpliedType()
	var path cty.Path
	val, err := unmarshal(value, ty, path)
	if err != nil {
		t.Fatalf("populating schema: %s", err)
	}
	return val
}

func unmarshal(value cty.Value, ty cty.Type, path cty.Path) (cty.Value, error) {
	switch {
	case ty.IsPrimitiveType():
		return value, nil
	// case ty.IsListType():
	// 	return unmarshalList(value, ty.ElementType(), path)
	case ty.IsSetType():
		return unmarshalSet(value, ty.ElementType(), path)
	case ty.IsMapType():
		return unmarshalMap(value, ty.ElementType(), path)
	// case ty.IsTupleType():
	// 	return unmarshalTuple(value, ty.TupleElementTypes(), path)
	case ty.IsObjectType():
		return unmarshalObject(value, ty.AttributeTypes(), path)
	default:
		return cty.NilVal, path.NewErrorf("unsupported type %s", ty.FriendlyName())
	}
}

func unmarshalSet(dec cty.Value, ety cty.Type, path cty.Path) (cty.Value, error) {
	if dec.IsNull() {
		return dec, nil
	}

	length := dec.LengthInt()

	if length == 0 {
		return cty.SetValEmpty(ety), nil
	}

	vals := make([]cty.Value, 0, length)
	dec.ForEachElement(func(key, val cty.Value) (stop bool) {
		vals = append(vals, val)
		return
	})

	return cty.SetVal(vals), nil
}

func unmarshalMap(dec cty.Value, ety cty.Type, path cty.Path) (cty.Value, error) {
	if dec.IsNull() {
		return dec, nil
	}

	length := dec.LengthInt()

	if length == 0 {
		return cty.MapValEmpty(ety), nil
	}

	vals := make(map[string]cty.Value, length)
	dec.ForEachElement(func(key, val cty.Value) (stop bool) {
		k := stringValue(key)
		vals[k] = val
		return
	})

	return cty.MapVal(vals), nil
}

func unmarshalObject(dec cty.Value, atys map[string]cty.Type, path cty.Path) (cty.Value, error) {
	if dec.IsNull() {
		return dec, nil
	}
	valueTy := dec.Type()

	vals := make(map[string]cty.Value, len(atys))
	path = append(path, nil)
	for key, aty := range atys {
		path[len(path)-1] = cty.IndexStep{
			Key: cty.StringVal(key),
		}

		if !valueTy.HasAttribute(key) {
			vals[key] = cty.NullVal(aty)
		} else {
			val, err := unmarshal(dec.GetAttr(key), aty, path)
			if err != nil {
				return cty.DynamicVal, err
			}
			vals[key] = val
		}
	}

	return cty.ObjectVal(vals), nil
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	} else {
		return v
	}
}
