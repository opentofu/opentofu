// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package s3

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

func TestValidateKMSKey(t *testing.T) {
	t.Parallel()

	path := cty.Path{cty.GetAttrStep{Name: "field"}}

	testcases := map[string]struct {
		in       string
		expected tfdiags.Diagnostics
	}{
		"kms key id": {
			in: "57ff7a43-341d-46b6-aee3-a450c9de6dc8",
		},
		"kms key arn": {
			in: "arn:aws:kms:us-west-2:111122223333:key/57ff7a43-341d-46b6-aee3-a450c9de6dc8",
		},
		"kms multi-region key id": {
			in: "mrk-f827515944fb43f9b902a09d2c8b554f",
		},
		"kms multi-region key arn": {
			in: "arn:aws:kms:us-west-2:111122223333:key/mrk-a835af0b39c94b86a21a8fc9535df681",
		},
		"kms key alias": {
			in: "alias/arbitrary-key",
		},
		"kms key alias arn": {
			in: "arn:aws:kms:us-west-2:111122223333:alias/arbitrary-key",
		},
		"invalid key": {
			in: "$%wrongkey",
			expected: tfdiags.Diagnostics{
				tfdiags.AttributeValue(
					tfdiags.Error,
					"Invalid KMS Key ID",
					`Value must be a valid KMS Key ID, got "$%wrongkey"`,
					path,
				),
			},
		},
		"non-kms arn": {
			in: "arn:aws:lambda:foo:bar:key/xyz",
			expected: tfdiags.Diagnostics{
				tfdiags.AttributeValue(
					tfdiags.Error,
					"Invalid KMS Key ARN",
					`Value must be a valid KMS Key ARN, got "arn:aws:lambda:foo:bar:key/xyz"`,
					path,
				),
			},
		},
	}

	for name, testcase := range testcases {
		testcase := testcase
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			diags := validateKMSKey(path, testcase.in)

			if diff := cmp.Diff(diags, testcase.expected, cmp.Comparer(diagnosticComparer)); diff != "" {
				t.Errorf("unexpected diagnostics difference: %s", diff)
			}
		})
	}
}

func TestValidateKeyARN(t *testing.T) {
	t.Parallel()

	path := cty.Path{cty.GetAttrStep{Name: "field"}}

	testcases := map[string]struct {
		in       string
		expected tfdiags.Diagnostics
	}{
		"kms key id": {
			in: "arn:aws:kms:us-west-2:123456789012:key/57ff7a43-341d-46b6-aee3-a450c9de6dc8",
		},
		"kms mrk key id": {
			in: "arn:aws:kms:us-west-2:111122223333:key/mrk-a835af0b39c94b86a21a8fc9535df681",
		},
		"kms non-key id": {
			in: "arn:aws:kms:us-west-2:123456789012:something/else",
			expected: tfdiags.Diagnostics{
				tfdiags.AttributeValue(
					tfdiags.Error,
					"Invalid KMS Key ARN",
					`Value must be a valid KMS Key ARN, got "arn:aws:kms:us-west-2:123456789012:something/else"`,
					path,
				),
			},
		},
		"non-kms arn": {
			in: "arn:aws:iam::123456789012:user/David",
			expected: tfdiags.Diagnostics{
				tfdiags.AttributeValue(
					tfdiags.Error,
					"Invalid KMS Key ARN",
					`Value must be a valid KMS Key ARN, got "arn:aws:iam::123456789012:user/David"`,
					path,
				),
			},
		},
		"not an arn": {
			in: "not an arn",
			expected: tfdiags.Diagnostics{
				tfdiags.AttributeValue(
					tfdiags.Error,
					"Invalid KMS Key ARN",
					`Value must be a valid KMS Key ARN, got "not an arn"`,
					path,
				),
			},
		},
	}

	for name, testcase := range testcases {
		testcase := testcase
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			diags := validateKMSKeyARN(path, testcase.in)

			if diff := cmp.Diff(diags, testcase.expected, cmp.Comparer(diagnosticComparer)); diff != "" {
				t.Errorf("unexpected diagnostics difference: %s", diff)
			}
		})
	}
}

func Test_validateAttributesConflict(t *testing.T) {
	tests := []struct {
		name      string
		paths     []cty.Path
		objValues map[string]cty.Value
		expectErr bool
	}{
		{
			name: "Conflict Found",
			paths: []cty.Path{
				{cty.GetAttrStep{Name: "attr1"}},
				{cty.GetAttrStep{Name: "attr2"}},
			},
			objValues: map[string]cty.Value{
				"attr1": cty.StringVal("value1"),
				"attr2": cty.StringVal("value2"),
				"attr3": cty.StringVal("value3"),
			},
			expectErr: true,
		},
		{
			name: "No Conflict",
			paths: []cty.Path{
				{cty.GetAttrStep{Name: "attr1"}},
				{cty.GetAttrStep{Name: "attr2"}},
			},
			objValues: map[string]cty.Value{
				"attr1": cty.StringVal("value1"),
				"attr2": cty.NilVal,
				"attr3": cty.StringVal("value3"),
			},
			expectErr: false,
		},
		{
			name: "Nested: Conflict Found",
			paths: []cty.Path{
				(cty.Path{cty.GetAttrStep{Name: "nested"}}).GetAttr("attr1"),
				{cty.GetAttrStep{Name: "attr2"}},
			},
			objValues: map[string]cty.Value{
				"nested": cty.ObjectVal(map[string]cty.Value{
					"attr1": cty.StringVal("value1"),
				}),
				"attr2": cty.StringVal("value2"),
				"attr3": cty.StringVal("value3"),
			},
			expectErr: true,
		},
		{
			name: "Nested: No Conflict",
			paths: []cty.Path{
				(cty.Path{cty.GetAttrStep{Name: "nested"}}).GetAttr("attr1"),
				{cty.GetAttrStep{Name: "attr3"}},
			},
			objValues: map[string]cty.Value{
				"nested": cty.NilVal,
				"attr1":  cty.StringVal("value1"),
				"attr3":  cty.StringVal("value3"),
			},
			expectErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var diags tfdiags.Diagnostics

			validator := validateAttributesConflict(test.paths...)

			obj := cty.ObjectVal(test.objValues)

			validator(obj, cty.Path{}, &diags)

			if test.expectErr {
				if !diags.HasErrors() {
					t.Error("Expected validation errors, but got none.")
				}
			} else {
				if diags.HasErrors() {
					t.Errorf("Expected no errors, but got %s.", diags.Err())
				}
			}
		})
	}
}

func Test_validateNestedAssumeRole(t *testing.T) {
	tests := []struct {
		description   string
		input         cty.Value
		expectedDiags []string
	}{
		{
			description: "Valid Input",
			input: cty.ObjectVal(map[string]cty.Value{
				"role_arn":     cty.StringVal("valid-role-arn"),
				"duration":     cty.StringVal("30m"),
				"external_id":  cty.StringVal("valid-external-id"),
				"policy":       cty.StringVal("valid-policy"),
				"session_name": cty.StringVal("valid-session-name"),
				"policy_arns":  cty.ListVal([]cty.Value{cty.StringVal("arn:aws:iam::123456789012:policy/valid-policy-arn")}),
			}),
			expectedDiags: nil,
		},
		{
			description: "Missing Role ARN",
			input: cty.ObjectVal(map[string]cty.Value{
				"role_arn":     cty.StringVal(""),
				"duration":     cty.StringVal("30m"),
				"external_id":  cty.StringVal("valid-external-id"),
				"policy":       cty.StringVal("valid-policy"),
				"session_name": cty.StringVal("valid-session-name"),
				"policy_arns":  cty.ListVal([]cty.Value{cty.StringVal("arn:aws:iam::123456789012:policy/valid-policy-arn")}),
			}),
			expectedDiags: []string{
				"The attribute \"assume_role.role_arn\" is required by the backend.\n\nRefer to the backend documentation for additional information which attributes are required.",
			},
		},
		{
			description: "Invalid Duration",
			input: cty.ObjectVal(map[string]cty.Value{
				"role_arn":     cty.StringVal("valid-role-arn"),
				"duration":     cty.StringVal("invalid-duration"),
				"external_id":  cty.StringVal("valid-external-id"),
				"policy":       cty.StringVal("valid-policy"),
				"session_name": cty.StringVal("valid-session-name"),
				"policy_arns":  cty.ListVal([]cty.Value{cty.StringVal("arn:aws:iam::123456789012:policy/valid-policy-arn")}),
			}),
			expectedDiags: []string{
				"The value \"invalid-duration\" cannot be parsed as a duration: time: invalid duration \"invalid-duration\"",
			},
		},
		{
			description: "Invalid Duration Length",
			input: cty.ObjectVal(map[string]cty.Value{
				"role_arn":     cty.StringVal("valid-role-arn"),
				"duration":     cty.StringVal("44h"),
				"external_id":  cty.StringVal("valid-external-id"),
				"policy":       cty.StringVal("valid-policy"),
				"session_name": cty.StringVal("valid-session-name"),
				"policy_arns":  cty.ListVal([]cty.Value{cty.StringVal("arn:aws:iam::123456789012:policy/valid-policy-arn")}),
			}),
			expectedDiags: []string{
				"Duration must be between 15m0s and 12h0m0s, had 44h",
			},
		},
		{
			description: "Invalid External ID (Empty)",
			input: cty.ObjectVal(map[string]cty.Value{
				"role_arn":     cty.StringVal("valid-role-arn"),
				"duration":     cty.StringVal("30m"),
				"external_id":  cty.StringVal(""),
				"policy":       cty.StringVal("valid-policy"),
				"session_name": cty.StringVal("valid-session-name"),
				"policy_arns":  cty.ListVal([]cty.Value{cty.StringVal("arn:aws:iam::123456789012:policy/valid-policy-arn")}),
			}),
			expectedDiags: []string{
				"The value cannot be empty or all whitespace",
			},
		},
		{
			description: "Invalid Policy (Empty)",
			input: cty.ObjectVal(map[string]cty.Value{
				"role_arn":     cty.StringVal("valid-role-arn"),
				"duration":     cty.StringVal("30m"),
				"external_id":  cty.StringVal("valid-external-id"),
				"policy":       cty.StringVal(""),
				"session_name": cty.StringVal("valid-session-name"),
				"policy_arns":  cty.ListVal([]cty.Value{cty.StringVal("arn:aws:iam::123456789012:policy/valid-policy-arn")}),
			}),
			expectedDiags: []string{
				"The value cannot be empty or all whitespace",
			},
		},
		{
			description: "Invalid Session Name (Empty)",
			input: cty.ObjectVal(map[string]cty.Value{
				"role_arn":     cty.StringVal("valid-role-arn"),
				"duration":     cty.StringVal("30m"),
				"external_id":  cty.StringVal("valid-external-id"),
				"policy":       cty.StringVal("valid-policy"),
				"session_name": cty.StringVal(""),
				"policy_arns":  cty.ListVal([]cty.Value{cty.StringVal("arn:aws:iam::123456789012:policy/valid-policy-arn")}),
			}),
			expectedDiags: []string{
				"The value cannot be empty or all whitespace",
			},
		},
		{
			description: "Invalid Policy ARN (Invalid ARN Format)",
			input: cty.ObjectVal(map[string]cty.Value{
				"role_arn":     cty.StringVal("valid-role-arn"),
				"duration":     cty.StringVal("30m"),
				"external_id":  cty.StringVal("valid-external-id"),
				"policy":       cty.StringVal("valid-policy"),
				"session_name": cty.StringVal("valid-session-name"),
				"policy_arns":  cty.ListVal([]cty.Value{cty.StringVal("invalid-arn-format")}),
			}),
			expectedDiags: []string{
				"The value [\"invalid-arn-format\"] cannot be parsed as an ARN: arn: invalid prefix",
			},
		},
		{
			description: "Invalid Policy ARN (Not Starting with 'policy/')",
			input: cty.ObjectVal(map[string]cty.Value{
				"role_arn":     cty.StringVal("valid-role-arn"),
				"duration":     cty.StringVal("30m"),
				"external_id":  cty.StringVal("valid-external-id"),
				"policy":       cty.StringVal("valid-policy"),
				"session_name": cty.StringVal("valid-session-name"),
				"policy_arns":  cty.ListVal([]cty.Value{cty.StringVal("arn:aws:iam::123456789012:role/invalid-policy-arn")}),
			}),
			expectedDiags: []string{
				"Value must be a valid IAM Policy ARN, got [\"arn:aws:iam::123456789012:role/invalid-policy-arn\"]",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			diagnostics := validateNestedAssumeRole(test.input, cty.Path{cty.GetAttrStep{Name: "assume_role"}})
			if len(diagnostics) != len(test.expectedDiags) {
				t.Errorf("Expected %d diagnostics, but got %d", len(test.expectedDiags), len(diagnostics))
			}
			for i, diag := range diagnostics {
				if diag.Description().Detail != test.expectedDiags[i] {
					t.Errorf("Mismatch in diagnostic %d. Expected: %q, Got: %q", i, test.expectedDiags[i], diag.Description().Detail)
				}
			}
		})
	}
}
