// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package aws_kms

import (
	"fmt"
	"log"
	"regexp"
)

type ConfigEndpoints struct {
	IAM string `hcl:"iam,optional"`
	STS string `hcl:"sts,optional"`
}

// Mirrored from s3 backend config
func includeProtoIfNecessary(endpoint string) string {
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
		endpoints = c.Endpoints[0]
	}
	if len(c.Endpoints) > 1 {
		return endpoints, fmt.Errorf("expected single aws_kms endpoints block, multiple provided")
	}

	// Endpoint formatting
	if len(endpoints.IAM) != 0 {
		endpoints.IAM = includeProtoIfNecessary(endpoints.IAM)
	}
	if len(endpoints.STS) != 0 {
		endpoints.STS = includeProtoIfNecessary(endpoints.STS)
	}
	return endpoints, nil
}
