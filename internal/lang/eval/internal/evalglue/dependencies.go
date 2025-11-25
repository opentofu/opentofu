// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package evalglue

import (
	"context"

	"github.com/apparentlymart/go-versions/versions"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Modules is implemented by callers of this package to provide access to
// the modules needed by a configuration without this package needing to
// know anything about how to fetch modules and perform the initial parsing
// and static decoding steps for them.
type ExternalModules interface {
	// ModuleConfig finds and loads a module meeting the given constraints.
	//
	// OpenTofu allows each module call to have a different version constraint
	// and selected module version, and so this signature also includes the
	// address of the module call the request is made on behalf of so that
	// the implementation can potentially use a lock file to determine which
	// version has been selected for that call in particular. forCall is
	// nil when requesting the root module.
	ModuleConfig(ctx context.Context, source addrs.ModuleSource, allowedVersions versions.Set, forCall *addrs.AbsModuleCall) (UncompiledModule, tfdiags.Diagnostics)
}

// Providers is implemented by callers of this package to provide access
// to the provider schemas needed by a configuration without this package needing
// to know anything about how provider plugins work, or whether plugins are
// even being used.
type ProvidersSchema interface {
	// ProviderConfigSchema returns the schema that should be used to evaluate
	// a "provider" block associated with the given provider.
	//
	// All providers are required to have a config schema, although for some
	// providers it is completely empty to represent that no explicit
	// configuration is needed.
	ProviderConfigSchema(ctx context.Context, provider addrs.Provider) (*providers.Schema, tfdiags.Diagnostics)

	// ResourceTypeSchema returns the schema for configuration and state of
	// a resource of the given type, or nil if the given provider does not
	// offer any such resource type.
	//
	// Returns error diagnostics if the given provider isn't available for use
	// at all, regardless of the resource type.
	ResourceTypeSchema(ctx context.Context, provider addrs.Provider, mode addrs.ResourceMode, typeName string) (*providers.Schema, tfdiags.Diagnostics)
}

// Providers is implemented by callers of this package to provide access
// to the provisioners needed by a configuration.
type ProvisionersSchema interface {
	// ProvisionerConfigSchema returns the schema that should be used to
	// evaluate a "provisioner" block associated with the given provisioner
	// type, or nil if there is no known provisioner of the given name.
	ProvisionerConfigSchema(ctx context.Context, typeName string) (*configschema.Block, tfdiags.Diagnostics)
}

// emptyDependencies is an implementation of all of our dependency-related
// interfaces at once, in all cases behaving as if nothing exists.
//
// We use this with [ensureExternalModules], [ensureProviders], and
// [ensureProvisioners] to substitute a caller-provided nil implementation
// with a non-nil implementation that contains nothing, so that the rest
// of the code doesn't need to repeatedly check for and handle nil.
//
// This returns low-quality error messages not suitable for use in real
// situations; it's here primarily for convenience when writing unit tests
// which don't make any use of a particular kind of dependency.
type emptyDependencies struct{}

func ensureExternalModules(given ExternalModules) ExternalModules {
	if given == nil {
		return emptyDependencies{}
	}
	return given
}

func ensureProviders(given ProvidersSchema) ProvidersSchema {
	if given == nil {
		return emptyDependencies{}
	}
	return given
}

func ensureProvisioners(given ProvisionersSchema) ProvisionersSchema {
	if given == nil {
		return emptyDependencies{}
	}
	return given
}

// ModuleConfig implements ExternalModules.
func (e emptyDependencies) ModuleConfig(ctx context.Context, source addrs.ModuleSource, allowedVersions versions.Set, forCall *addrs.AbsModuleCall) (UncompiledModule, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	diags = diags.Append(tfdiags.Sourceless(
		tfdiags.Error,
		"No modules are available",
		"There are no modules available for use in this context.",
	))
	return nil, diags
}

// ProviderConfigSchema implements Providers.
func (e emptyDependencies) ProviderConfigSchema(ctx context.Context, provider addrs.Provider) (*providers.Schema, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	diags = diags.Append(tfdiags.Sourceless(
		tfdiags.Error,
		"No providers are available",
		"There are no providers available for use in this context.",
	))
	return nil, diags
}

// ResourceTypeSchema implements Providers.
func (e emptyDependencies) ResourceTypeSchema(ctx context.Context, provider addrs.Provider, mode addrs.ResourceMode, typeName string) (*providers.Schema, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	diags = diags.Append(tfdiags.Sourceless(
		tfdiags.Error,
		"No providers are available",
		"There are no providers available for use in this context.",
	))
	return nil, diags
}

// ProvisionerConfigSchema implements Provisioners.
func (e emptyDependencies) ProvisionerConfigSchema(ctx context.Context, typeName string) (*configschema.Block, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	diags = diags.Append(tfdiags.Sourceless(
		tfdiags.Error,
		"No provisioners are available",
		"There are no provisioners available for use in this context.",
	))
	return nil, diags
}
