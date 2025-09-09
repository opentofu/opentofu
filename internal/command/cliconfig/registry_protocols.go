// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cliconfig

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/hashicorp/hcl"
	hclast "github.com/hashicorp/hcl/hcl/ast"
	hcltoken "github.com/hashicorp/hcl/hcl/token"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

// RegistryProtocolsConfig models the "registry_protocols" configuration block
// and its associated environment variables.
//
// These settings are a little awkward in that they relate to OpenTofu's native
// module and provider registry protocols, but not to any other module or
// provider installation methods. The name of this block type assumes that
// we'd extend these settings to any other OpenTofu-native registry protocols
// we might add in future, although at the time of writing this comment we have
// no plans to introduce any, since we're preferring to use industry-standard
// artifact distribution protocols like OCI Distribution.
//
// They also, due to a historical implementation mistake that we preserved for
// backward-compatibility, also partially influence OpenTofu's native service
// discovery client regardless of which service protocol it's trying to perform
// discovery for.
type RegistryProtocolsConfig struct {
	// RetryCount specifies the number of times OpenTofu should
	// retry making metadata requests to module or provider registries
	// when it encounters a retryable error.
	RetryCount    int
	RetryCountSet bool // for tracking overrides between files

	// RequestTimeout is the amount of time to wait for a response
	// to a metadata request to a module or provider registry.
	RequestTimeout    time.Duration
	RequestTimeoutSet bool // for tracking overrides between files
}

func decodeRegistryProtocolsConfigFromConfig(hclFile *hclast.File) (*RegistryProtocolsConfig, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	var ret *RegistryProtocolsConfig
	var retPos hcltoken.Pos

	root := hclFile.Node.(*hclast.ObjectList)
	for _, block := range root.Items {
		if block.Keys[0].Token.Value() != "registry_protocols" {
			continue
		}
		if ret != nil {
			// We don't allow multiple registry_protocols blocks in the same
			// file because that would be confusing and should never be
			// necessary, but note that we _do_ allow different files (or
			// other sources like the environment) to each have their own
			// block and then our caller is responsible for merging them.
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Duplicate registry_protocols block",
				fmt.Sprintf("The registry protocol settings were already defined at %s.", retPos),
			))
			continue
		}

		body, ok := block.Val.(*hclast.ObjectType)
		if !ok {
			// We'll get here if the config contains something particularly
			// weird, such as: registry_protocols = "not_an_object"
			//
			// HCL 1 makes it hard to handle all of these variations robustly
			// with good error messages because its AST can have many different
			// shapes depending on what the author wrote, and so we'll just
			// accept this generic generic error message and hope this won't
			// arise often because people will follow the examples we show
			// in our documentation.
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid registry_protocols block",
				fmt.Sprintf("The registry_protocols item at %s must have an open brace after the block type.", block.Pos()),
			))
			continue
		}

		config, moreDiags := decodeRegistryProtocolsConfigFromConfigBody(body)
		diags = diags.Append(moreDiags)
		ret = config
		retPos = block.Pos()
	}

	return ret, diags
}

func decodeRegistryProtocolsConfigFromConfigBody(body *hclast.ObjectType) (*RegistryProtocolsConfig, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	ret := &RegistryProtocolsConfig{
		// By the time we get here we definitely have a block, regardless of
		// whether anything is set in it. We'll populate this selectively below.
	}

	type BodyContent struct {
		RetryCount     *int `hcl:"retry_count"`
		RequestTimeout *int `hcl:"request_timeout_seconds"`
	}
	var bodyContent BodyContent
	err := hcl.DecodeObject(&bodyContent, body)
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid registry_protocols block",
			fmt.Sprintf("Invalid registry protocol settings at %s: %s.", body.Pos(), err),
		))
		return ret, diags
	}
	if bodyContent.RetryCount != nil {
		ret.RetryCount = *bodyContent.RetryCount
		ret.RetryCountSet = true
	}
	if bodyContent.RequestTimeout != nil {
		ret.RequestTimeout = time.Duration(*bodyContent.RequestTimeout) * time.Second
		ret.RequestTimeoutSet = true
	}
	return ret, diags
}

// decodeRegistryProtocolsConfigFromEnvironment returns a
// [RegistryProtocolsConfig] object describing a "virtual" registry_protocols
// configuration block implied by environment variables, if any are set.
func decodeRegistryProtocolsConfigFromEnvironment() *RegistryProtocolsConfig {
	ret := RegistryProtocolsConfig{}
	if v := os.Getenv(registryDiscoveryRetryEnvName); v != "" {
		override, err := strconv.Atoi(v)
		if err == nil && override > 0 {
			ret.RetryCount = override
			ret.RetryCountSet = true
		}
	}
	if v := os.Getenv(registryClientTimeoutEnvName); v != "" {
		override, err := strconv.Atoi(v)
		if err == nil && override > 0 {
			ret.RequestTimeout = time.Duration(override) * time.Second
			ret.RequestTimeoutSet = true
		}
	}
	if !ret.RetryCountSet && !ret.RequestTimeoutSet {
		// If neither variable is set then we behave as if this virtual
		// configuration block is not present at all.
		return nil
	}
	return &ret
}

// mergeRegistryProtocolConfigs is used by [Config.Merge] for merging registry
// protocol configurations from multiple different files.
func mergeRegistryProtocolConfigs(base, override *RegistryProtocolsConfig) *RegistryProtocolsConfig {
	// We only have work to do here if both arguments are set.
	if base == nil {
		return override
	}
	if override == nil {
		return base
	}

	// If we're going to change anything then we must always produce a new
	// object, because the caller expects both of the given objects to
	// remain unmodified.
	ret := &RegistryProtocolsConfig{}
	if override.RetryCountSet {
		ret.RetryCount = override.RetryCount
		ret.RetryCountSet = override.RetryCountSet
	} else {
		ret.RetryCount = base.RetryCount
		ret.RetryCountSet = base.RetryCountSet
	}
	if override.RequestTimeoutSet {
		ret.RequestTimeout = override.RequestTimeout
		ret.RequestTimeoutSet = override.RequestTimeoutSet
	} else {
		ret.RequestTimeout = base.RequestTimeout
		ret.RequestTimeoutSet = base.RequestTimeoutSet
	}
	return ret
}

func init() {
	// BuiltinConfig should contain the default values for a registry_protocols
	// block.
	BuiltinConfig.RegistryProtocols = &RegistryProtocolsConfig{
		RetryCount:        registryDiscoveryDefaultRetryCount,
		RetryCountSet:     true,
		RequestTimeout:    registryClientDefaultRequestTimeout,
		RequestTimeoutSet: true,
	}
}

const (
	// registryDiscoveryRetryEnvName is the name of the environment variable that
	// can be configured to customize number of retries for module and provider
	// discovery requests with the remote registry.
	registryDiscoveryRetryEnvName      = "TF_REGISTRY_DISCOVERY_RETRY"
	registryDiscoveryDefaultRetryCount = 1

	// registryClientTimeoutEnvName is the name of the environment variable that
	// can be configured to customize the timeout duration (seconds) for module
	// and provider discovery with a remote registry. For historical reasons
	// this also applies to all service discovery requests regardless of whether
	// they are registry-related.
	registryClientTimeoutEnvName        = "TF_REGISTRY_CLIENT_TIMEOUT"
	registryClientDefaultRequestTimeout = 10 * time.Second
)
