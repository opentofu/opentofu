// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package aws_kms

import (
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	awsbase "github.com/hashicorp/aws-sdk-go-base/v2"
)

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

func (r *AssumeRole) asAWSBase() (*awsbase.AssumeRole, error) {
	if r == nil {
		return nil, nil
	}

	duration, err := parseAssumeRoleDuration(r.Duration)
	if err != nil {
		return nil, err
	}

	err = validatePolicyARNs(r.PolicyARNs)
	if err != nil {
		return nil, err
	}

	assumeRole := &awsbase.AssumeRole{
		RoleARN:           r.RoleARN,
		Duration:          duration,
		ExternalID:        r.ExternalID,
		Policy:            r.Policy,
		PolicyARNs:        r.PolicyARNs,
		SessionName:       r.SessionName,
		Tags:              r.Tags,
		TransitiveTagKeys: r.TransitiveTagKeys,
	}
	return assumeRole, nil
}
func (r *AssumeRoleWithWebIdentity) asAWSBase() (*awsbase.AssumeRoleWithWebIdentity, error) {
	if r == nil {
		return nil, nil
	}

	if r.WebIdentityToken != "" && r.WebIdentityTokenFile != "" {
		return nil, fmt.Errorf("conflicting config attributes: only web_identity_token or web_identity_token_file can be specified, not both")
	}

	duration, err := parseAssumeRoleDuration(r.Duration)
	if err != nil {
		return nil, err
	}

	err = validatePolicyARNs(r.PolicyARNs)
	if err != nil {
		return nil, err
	}

	return &awsbase.AssumeRoleWithWebIdentity{
		RoleARN:              stringAttrEnvFallback(r.RoleARN, "AWS_ROLE_ARN"),
		Duration:             duration,
		Policy:               r.Policy,
		PolicyARNs:           r.PolicyARNs,
		SessionName:          stringAttrEnvFallback(r.SessionName, "AWS_ROLE_SESSION_NAME"),
		WebIdentityToken:     stringAttrEnvFallback(r.WebIdentityToken, "AWS_WEB_IDENTITY_TOKEN"),
		WebIdentityTokenFile: stringAttrEnvFallback(r.WebIdentityTokenFile, "AWS_WEB_IDENTITY_TOKEN_FILE"),
	}, nil
}
