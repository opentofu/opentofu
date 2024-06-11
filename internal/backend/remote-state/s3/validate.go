// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package s3

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

const (
	multiRegionKeyIdPattern = `mrk-[a-f0-9]{32}`
	uuidRegexPattern        = `[a-f0-9]{8}-[a-f0-9]{4}-[1-5][a-f0-9]{3}-[ab89][a-f0-9]{3}-[a-f0-9]{12}`
	aliasRegexPattern       = `alias/[a-zA-Z0-9/_-]+`
)

func validateKMSKey(path cty.Path, s string) (diags tfdiags.Diagnostics) {
	if arn.IsARN(s) {
		return validateKMSKeyARN(path, s)
	}
	return validateKMSKeyID(path, s)
}

func validateKMSKeyID(path cty.Path, s string) (diags tfdiags.Diagnostics) {
	keyIdRegex := regexp.MustCompile(`^` + uuidRegexPattern + `|` + multiRegionKeyIdPattern + `|` + aliasRegexPattern + `$`)
	if !keyIdRegex.MatchString(s) {
		diags = diags.Append(tfdiags.AttributeValue(
			tfdiags.NewSeverity(tfdiags.ErrorLevel),
			"Invalid KMS Key ID",
			fmt.Sprintf("Value must be a valid KMS Key ID, got %q", s),
			path,
		))
		return diags
	}

	return diags
}

func validateKMSKeyARN(path cty.Path, s string) (diags tfdiags.Diagnostics) {
	parsedARN, err := arn.Parse(s)
	if err != nil {
		diags = diags.Append(tfdiags.AttributeValue(
			tfdiags.NewSeverity(tfdiags.ErrorLevel),
			"Invalid KMS Key ARN",
			fmt.Sprintf("Value must be a valid KMS Key ARN, got %q", s),
			path,
		))
		return diags
	}

	if !isKeyARN(parsedARN) {
		diags = diags.Append(tfdiags.AttributeValue(
			tfdiags.NewSeverity(tfdiags.ErrorLevel),
			"Invalid KMS Key ARN",
			fmt.Sprintf("Value must be a valid KMS Key ARN, got %q", s),
			path,
		))
		return diags
	}

	return diags
}

func validateNestedAssumeRole(obj cty.Value, objPath cty.Path) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	if val, ok := stringAttrOk(obj, "role_arn"); !ok || val == "" {
		path := objPath.GetAttr("role_arn")
		diags = diags.Append(attributeErrDiag(
			"Missing Required Value",
			fmt.Sprintf("The attribute %q is required by the backend.\n\n", pathString(path))+
				"Refer to the backend documentation for additional information which attributes are required.",
			path,
		))
	}

	if val, ok := stringAttrOk(obj, "duration"); ok {
		validateDuration(val, 15*time.Minute, 12*time.Hour, objPath.GetAttr("duration"), &diags)
	}

	if val, ok := stringAttrOk(obj, "external_id"); ok {
		validateNonEmptyString(val, objPath.GetAttr("external_id"), &diags)
	}

	if val, ok := stringAttrOk(obj, "policy"); ok {
		validateNonEmptyString(val, objPath.GetAttr("policy"), &diags)
	}

	if val, ok := stringAttrOk(obj, "session_name"); ok {
		validateNonEmptyString(val, objPath.GetAttr("session_name"), &diags)
	}

	if val, ok := stringSliceAttrOk(obj, "policy_arns"); ok {
		validatePolicyARNSlice(val, objPath.GetAttr("policy_arns"), &diags)
	}

	return diags
}

func validateAssumeRoleWithWebIdentity(obj cty.Value, objPath cty.Path) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	validateAttributesConflict(
		cty.GetAttrPath("web_identity_token"),
		cty.GetAttrPath("web_identity_token_file"),
	)(obj, objPath, &diags)

	if val, ok := stringAttrOk(obj, "session_name"); ok {
		validateNonEmptyString(val, objPath.GetAttr("session_name"), &diags)
	}

	if val, ok := stringAttrOk(obj, "policy"); ok {
		validateNonEmptyString(val, objPath.GetAttr("policy"), &diags)
	}

	if val, ok := stringSliceAttrOk(obj, "policy_arns"); ok {
		validatePolicyARNSlice(val, objPath.GetAttr("policy_arns"), &diags)
	}

	if val, ok := stringAttrOk(obj, "duration"); ok {
		validateDuration(val, 15*time.Minute, 12*time.Hour, objPath.GetAttr("duration"), &diags)
	}

	return diags
}

func isKeyARN(arn arn.ARN) bool {
	return keyIdFromARNResource(arn.Resource) != "" || aliasIdFromARNResource(arn.Resource) != ""
}

func keyIdFromARNResource(s string) string {
	keyIdResourceRegex := regexp.MustCompile(`^key/(` + uuidRegexPattern + `|` + multiRegionKeyIdPattern + `)$`)
	matches := keyIdResourceRegex.FindStringSubmatch(s)
	if matches == nil || len(matches) != 2 {
		return ""
	}

	return matches[1]
}

func aliasIdFromARNResource(s string) string {
	aliasIdResourceRegex := regexp.MustCompile(`^(` + aliasRegexPattern + `)$`)
	matches := aliasIdResourceRegex.FindStringSubmatch(s)
	if matches == nil || len(matches) != 2 {
		return ""
	}

	return matches[1]
}

type objectValidator func(obj cty.Value, objPath cty.Path, diags *tfdiags.Diagnostics)

func validateAttributesConflict(paths ...cty.Path) objectValidator {
	applyPath := func(obj cty.Value, path cty.Path) (cty.Value, error) {
		if len(path) == 0 {
			return cty.NilVal, nil
		}
		for _, step := range path {
			val, err := step.Apply(obj)
			if err != nil {
				return cty.NilVal, err
			}
			if val.IsNull() {
				return cty.NilVal, nil
			}
			obj = val
		}
		return obj, nil
	}

	return func(obj cty.Value, objPath cty.Path, diags *tfdiags.Diagnostics) {
		found := false
		for _, path := range paths {
			val, err := applyPath(obj, path)
			if err != nil {
				*diags = diags.Append(attributeErrDiag(
					"Invalid Path for Schema",
					"The S3 Backend unexpectedly provided a path that does not match the schema. "+
						"Please report this to the developers.\n\n"+
						"Path: "+pathString(path)+"\n\n"+
						"Error: "+err.Error(),
					objPath,
				))
				continue
			}
			if !val.IsNull() {
				if found {
					pathStrs := make([]string, len(paths))
					for i, path := range paths {
						pathStrs[i] = pathString(path)
					}
					*diags = diags.Append(attributeErrDiag(
						"Invalid Attribute Combination",
						fmt.Sprintf(`Only one of %s can be set.`, strings.Join(pathStrs, ", ")),
						objPath,
					))
					return
				}
				found = true
			}
		}
	}
}

func attributeErrDiag(summary, detail string, attrPath cty.Path) tfdiags.Diagnostic {
	return tfdiags.AttributeValue(tfdiags.NewSeverity(tfdiags.ErrorLevel), summary, detail, attrPath.Copy())
}

func attributeWarningDiag(summary, detail string, attrPath cty.Path) tfdiags.Diagnostic {
	return tfdiags.AttributeValue(tfdiags.NewSeverity(tfdiags.WarningLevel), summary, detail, attrPath.Copy())
}

func validateNonEmptyString(val string, path cty.Path, diags *tfdiags.Diagnostics) {
	if len(strings.TrimSpace(val)) == 0 {
		*diags = diags.Append(attributeErrDiag(
			"Invalid Value",
			"The value cannot be empty or all whitespace",
			path,
		))
	}
}

func validatePolicyARNSlice(val []string, path cty.Path, diags *tfdiags.Diagnostics) {
	for _, v := range val {
		arn, err := arn.Parse(v)
		if err != nil {
			*diags = diags.Append(attributeErrDiag(
				"Invalid ARN",
				fmt.Sprintf("The value %q cannot be parsed as an ARN: %s", val, err),
				path,
			))
			break
		} else {
			if !strings.HasPrefix(arn.Resource, "policy/") {
				*diags = diags.Append(attributeErrDiag(
					"Invalid IAM Policy ARN",
					fmt.Sprintf("Value must be a valid IAM Policy ARN, got %q", val),
					path,
				))
			}
		}
	}
}

func validateDuration(val string, min, max time.Duration, path cty.Path, diags *tfdiags.Diagnostics) {
	d, err := time.ParseDuration(val)
	if err != nil {
		*diags = diags.Append(attributeErrDiag(
			"Invalid Duration",
			fmt.Sprintf("The value %q cannot be parsed as a duration: %s", val, err),
			path,
		))
		return
	}
	if (min > 0 && d < min) || (max > 0 && d > max) {
		*diags = diags.Append(attributeErrDiag(
			"Invalid Duration",
			fmt.Sprintf("Duration must be between %s and %s, had %s", min, max, val),
			path,
		))
	}
}
