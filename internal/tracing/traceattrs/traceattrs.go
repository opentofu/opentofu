// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package traceattrs

const (
	// Common attributes names used across the codebase

	ProviderAddress = "opentofu.provider.address"
	ProviderVersion = "opentofu.provider.version"

	TargetPlatform = "opentofu.target_platform"

	ModuleCallName = "opentofu.module.name"
	ModuleSource   = "opentofu.module.source"
	ModuleVersion  = "opentofu.module.version"
)
