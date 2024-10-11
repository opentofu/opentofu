// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"fmt"
	"strings"

	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// ProviderInstance is an interface type whose dynamic type can be either
// LocalProviderInstance or ConfigProviderInstance, in order to represent
// situations where a value might either be module-local or absolute but the
// decision cannot be made until runtime.
//
// Where possible, use either LocalProviderInstance or ConfigProviderInstance
// directly instead, to make intent more clear. ProviderInstance can be used only
// in situations where the recipient of the value has some out-of-band way to
// determine a "current module" to use if the value turns out to be
// a LocalProviderInstance.
//
// Recipients of non-nil ProviderInstance values that actually need
// ConfigProviderInstance values should call ResolveAbsProviderAddr on the
// *configs.Config value representing the root module configuration, which
// handles the translation from local to fully-qualified using mapping tables
// defined in the configuration.
//
// Recipients of a ProviderInstance value can assume it can contain only a
// LocalProviderInstance value, an ConfigProviderInstance value, or nil to
// represent the absence of a provider config in situations where that is
// meaningful.
type ProviderInstance interface {
	providerInstance()
}

// LocalProviderInstance is the address of a provider configuration from the
// perspective of references in a particular module.
//
// Finding the corresponding ConfigProviderInstance will require looking up the
// LocalName in the providers table in the module's configuration; there is
// no syntax-only translation between these types.
type LocalProviderInstance struct {
	LocalName string

	// If not empty, Alias identifies which non-default (aliased) provider
	// configuration this address refers to.
	Alias string
}

var _ ProviderInstance = LocalProviderInstance{}
var _ Referenceable = LocalProviderInstance{}

// NewDefaultLocalProviderInstance returns the address of the default (un-aliased)
// configuration for the provider with the given local type name.
func NewDefaultLocalProviderInstance(LocalNameName string) LocalProviderInstance {
	return LocalProviderInstance{
		LocalName: LocalNameName,
	}
}

// providerInstance Implements addrs.ProviderInstance.
func (pc LocalProviderInstance) providerInstance() {}

func (pc LocalProviderInstance) String() string {
	if pc.LocalName == "" {
		// Should never happen; always indicates a bug
		return "provider.<invalid>"
	}

	if pc.Alias != "" {
		return fmt.Sprintf("provider.%s.%s", pc.LocalName, pc.Alias)
	}

	return "provider." + pc.LocalName
}

// StringCompact is an alternative to String that returns the compact form
// without the "provider." prefix.
func (pc LocalProviderInstance) StringCompact() string {
	if pc.Alias != "" {
		return fmt.Sprintf("%s.%s", pc.LocalName, pc.Alias)
	}
	return pc.LocalName
}

// UniqueKey implements UniqueKeyer and Referenceable.
func (pc LocalProviderInstance) UniqueKey() UniqueKey {
	// A LocalProviderInstance can be its own UniqueKey
	return pc
}

// uniqueKeySigil implements UniqueKey.
func (pc LocalProviderInstance) uniqueKeySigil() {}

// referenceableSigil implements Referenceable.
func (pc LocalProviderInstance) referenceableSigil() {}

// ConfigProviderInstance is the address of a provider block in the configuration,
// before any module expansion although already including a statically-configured
// "alias".
//
// In earlier versions this was called "AbsProviderConfig", but that was a misnomer
// because the "Abs..." prefix is for addresses that include a [ModuleInstance],
// not those that include a [Module].
type ConfigProviderInstance struct {
	Module   Module
	Provider Provider
	Alias    string
}

var _ ProviderInstance = ConfigProviderInstance{}

// ParseConfigProviderInstance parses the given traversal as a module-path-sensitive
// provider configuration address. The following are examples of traversals that can be
// successfully parsed as provider configuration addresses using this function:
//
//   - provider["registry.opentofu.org/hashicorp/aws"]
//   - provider["registry.opentofu.org/hashicorp/aws"].foo
//   - module.bar.provider["registry.opentofu.org/hashicorp/aws"]
//   - module.bar.module.baz.provider["registry.opentofu.org/hashicorp/aws"].foo
//
// This type of address is used, for example, to record the relationships
// between resources and provider configurations in the state structure.
// This type of address is typically not used prominently in the UI, except in
// error messages that refer to provider configurations.
func ParseConfigProviderInstance(traversal hcl.Traversal) (ConfigProviderInstance, tfdiags.Diagnostics) {
	modInst, remain, diags := parseModuleInstancePrefix(traversal)
	var ret ConfigProviderInstance

	// Providers cannot resolve within module instances, so verify that there
	// are no instance keys in the module path before converting to a Module.
	for _, step := range modInst {
		if step.InstanceKey != NoKey {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid provider configuration address",
				Detail:   "Provider address cannot contain module indexes",
				Subject:  remain.SourceRange().Ptr(),
			})
			return ret, diags
		}
	}
	ret.Module = modInst.Module()

	if len(remain) < 2 || remain.RootName() != "provider" {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid provider configuration address",
			Detail:   "Provider address must begin with \"provider.\", followed by a provider type name.",
			Subject:  remain.SourceRange().Ptr(),
		})
		return ret, diags
	}
	if len(remain) > 3 {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid provider configuration address",
			Detail:   "Extraneous operators after provider configuration alias.",
			Subject:  hcl.Traversal(remain[3:]).SourceRange().Ptr(),
		})
		return ret, diags
	}

	if tt, ok := remain[1].(hcl.TraverseIndex); ok {
		if !tt.Key.Type().Equals(cty.String) {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid provider configuration address",
				Detail:   "The prefix \"provider.\" must be followed by a provider type name.",
				Subject:  remain[1].SourceRange().Ptr(),
			})
			return ret, diags
		}
		p, sourceDiags := ParseProviderSourceString(tt.Key.AsString())
		ret.Provider = p
		if sourceDiags.HasErrors() {
			diags = diags.Append(sourceDiags)
			return ret, diags
		}
	} else {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid provider configuration address",
			Detail:   "The prefix \"provider.\" must be followed by a provider type name.",
			Subject:  remain[1].SourceRange().Ptr(),
		})
		return ret, diags
	}

	if len(remain) == 3 {
		if tt, ok := remain[2].(hcl.TraverseAttr); ok {
			ret.Alias = tt.Name
		} else {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid provider configuration address",
				Detail:   "Provider type name must be followed by a configuration alias name.",
				Subject:  remain[2].SourceRange().Ptr(),
			})
			return ret, diags
		}
	}

	return ret, diags
}

// ParseConfigProviderInstanceStr is a helper wrapper around ParseConfigProviderInstance
// that takes a string and parses it with the HCL native syntax traversal parser
// before interpreting it.
//
// This should be used only in specialized situations since it will cause the
// created references to not have any meaningful source location information.
// If a reference string is coming from a source that should be identified in
// error messages then the caller should instead parse it directly using a
// suitable function from the HCL API and pass the traversal itself to
// ParseConfigProviderInstance.
//
// Error diagnostics are returned if either the parsing fails or the analysis
// of the traversal fails. There is no way for the caller to distinguish the
// two kinds of diagnostics programmatically. If error diagnostics are returned
// the returned address is invalid.
func ParseConfigProviderInstanceStr(str string) (ConfigProviderInstance, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	traversal, parseDiags := hclsyntax.ParseTraversalAbs([]byte(str), "", hcl.Pos{Line: 1, Column: 1})
	diags = diags.Append(parseDiags)
	if parseDiags.HasErrors() {
		return ConfigProviderInstance{}, diags
	}
	addr, addrDiags := ParseConfigProviderInstance(traversal)
	diags = diags.Append(addrDiags)
	return addr, diags
}

func ParseLegacyConfigProviderInstanceStr(str string) (ConfigProviderInstance, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	traversal, parseDiags := hclsyntax.ParseTraversalAbs([]byte(str), "", hcl.Pos{Line: 1, Column: 1})
	diags = diags.Append(parseDiags)
	if parseDiags.HasErrors() {
		return ConfigProviderInstance{}, diags
	}

	addr, addrDiags := ParseLegacyConfigProviderInstance(traversal)
	diags = diags.Append(addrDiags)
	return addr, diags
}

// ParseLegacyConfigProviderInstance parses the given traversal as an absolute
// provider address in the legacy form used by OpenTofu v0.12 and earlier.
// The following are examples of traversals that can be successfully parsed as
// legacy absolute provider configuration addresses:
//
//   - provider.aws
//   - provider.aws.foo
//   - module.bar.provider.aws
//   - module.bar.module.baz.provider.aws.foo
//
// We can encounter this kind of address in a historical state snapshot that
// hasn't yet been upgraded by refreshing or applying a plan with
// OpenTofu v0.13. Later versions of OpenTofu reject state snapshots using
// this format, and so users must follow the OpenTofu v0.13 upgrade guide
// in that case.
//
// We will not use this address form for any new file formats.
func ParseLegacyConfigProviderInstance(traversal hcl.Traversal) (ConfigProviderInstance, tfdiags.Diagnostics) {
	modInst, remain, diags := parseModuleInstancePrefix(traversal)
	var ret ConfigProviderInstance

	// Providers cannot resolve within module instances, so verify that there
	// are no instance keys in the module path before converting to a Module.
	for _, step := range modInst {
		if step.InstanceKey != NoKey {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid provider configuration address",
				Detail:   "Provider address cannot contain module indexes",
				Subject:  remain.SourceRange().Ptr(),
			})
			return ret, diags
		}
	}
	ret.Module = modInst.Module()

	if len(remain) < 2 || remain.RootName() != "provider" {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid provider configuration address",
			Detail:   "Provider address must begin with \"provider.\", followed by a provider type name.",
			Subject:  remain.SourceRange().Ptr(),
		})
		return ret, diags
	}
	if len(remain) > 3 {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid provider configuration address",
			Detail:   "Extraneous operators after provider configuration alias.",
			Subject:  hcl.Traversal(remain[3:]).SourceRange().Ptr(),
		})
		return ret, diags
	}

	// We always assume legacy-style providers in legacy state ...
	if tt, ok := remain[1].(hcl.TraverseAttr); ok {
		// ... unless it's the builtin "terraform" provider, a special case.
		if tt.Name == "terraform" {
			ret.Provider = NewBuiltInProvider(tt.Name)
		} else {
			ret.Provider = NewLegacyProvider(tt.Name)
		}
	} else {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid provider configuration address",
			Detail:   "The prefix \"provider.\" must be followed by a provider type name.",
			Subject:  remain[1].SourceRange().Ptr(),
		})
		return ret, diags
	}

	if len(remain) == 3 {
		if tt, ok := remain[2].(hcl.TraverseAttr); ok {
			ret.Alias = tt.Name
		} else {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid provider configuration address",
				Detail:   "Provider type name must be followed by a configuration alias name.",
				Subject:  remain[2].SourceRange().Ptr(),
			})
			return ret, diags
		}
	}

	return ret, diags
}

// ProviderConfigDefault returns the address of the default provider config of
// the given type inside the receiving module instance.
func (m ModuleInstance) ProviderConfigDefault(provider Provider) ConfigProviderInstance {
	return ConfigProviderInstance{
		Module:   m.Module(),
		Provider: provider,
	}
}

// ProviderConfigAliased returns the address of an aliased provider config of
// the given type and alias inside the receiving module instance.
func (m ModuleInstance) ProviderConfigAliased(provider Provider, alias string) ConfigProviderInstance {
	return ConfigProviderInstance{
		Module:   m.Module(),
		Provider: provider,
		Alias:    alias,
	}
}

// providerInstance Implements addrs.ProviderConfig.
func (pc ConfigProviderInstance) providerInstance() {}

// Inherited returns an address that the receiving configuration address might
// inherit from in a parent module. The second bool return value indicates if
// such inheritance is possible, and thus whether the returned address is valid.
//
// Inheritance is possible only for default (un-aliased) providers in modules
// other than the root module. Even if a valid address is returned, inheritance
// may not be performed for other reasons, such as if the calling module
// provided explicit provider configurations within the call for this module.
// The ProviderInstanceTransformer graph transform in the main tofu module has the
// authoritative logic for provider inheritance, and this method is here mainly
// just for its benefit.
func (pc ConfigProviderInstance) Inherited() (ConfigProviderInstance, bool) {
	// Can't inherit if we're already in the root.
	if len(pc.Module) == 0 {
		return ConfigProviderInstance{}, false
	}

	// Can't inherit if we have an alias.
	if pc.Alias != "" {
		return ConfigProviderInstance{}, false
	}

	// Otherwise, we might inherit from a configuration with the same
	// provider type in the parent module instance.
	parentMod := pc.Module.Parent()
	return ConfigProviderInstance{
		Module:   parentMod,
		Provider: pc.Provider,
	}, true

}

// LegacyString() returns a legacy-style ConfigProviderInstance string and should only be used for legacy state shimming.
func (pc ConfigProviderInstance) LegacyString() string {
	if pc.Alias != "" {
		if len(pc.Module) == 0 {
			return fmt.Sprintf("%s.%s.%s", "provider", pc.Provider.LegacyString(), pc.Alias)
		} else {
			return fmt.Sprintf("%s.%s.%s.%s", pc.Module.String(), "provider", pc.Provider.LegacyString(), pc.Alias)
		}
	}
	if len(pc.Module) == 0 {
		return fmt.Sprintf("%s.%s", "provider", pc.Provider.LegacyString())
	}
	return fmt.Sprintf("%s.%s.%s", pc.Module.String(), "provider", pc.Provider.LegacyString())
}

// String() returns a string representation of a ConfigProviderInstance in a format like the following examples:
//
//   - provider["example.com/namespace/name"]
//   - provider["example.com/namespace/name"].alias
//   - module.module-name.provider["example.com/namespace/name"]
//   - module.module-name.provider["example.com/namespace/name"].alias
func (pc ConfigProviderInstance) String() string {
	var parts []string
	if len(pc.Module) > 0 {
		parts = append(parts, pc.Module.String())
	}

	parts = append(parts, fmt.Sprintf("provider[%q]", pc.Provider))

	if pc.Alias != "" {
		parts = append(parts, pc.Alias)
	}

	return strings.Join(parts, ".")
}

// AbsProviderInstance represents the fully-qualified of an instance of a
// provider, after instance expansion is complete.
//
// Each "provider" block in the configuration can become zero or more
// AbsProviderInstance after expansion, and all of the expanded instances
// must have unique AbsProviderInstance addresses.
type AbsProviderInstance struct {
	Module   ModuleInstance
	Provider Provider

	// Key is the instance key of an additional (aka "aliased") provider
	// instance. This is populated either from the "alias" argument of
	// the associated provider configuration, or from one of the keys
	// in the for_each argument.
	//
	// Unlike most other multi-instance address types, the key for a
	// provider instance is currently always either NoKey or a string,
	// and a string key always contains a valid HCL identifier. However,
	// best to try to avoid depending on those constraints as much as
	// possible in other code and in new language features so that we
	// can potentially generalize this more later if someone finds a
	// compelling use-case for making this behave more like other
	// expandable objects.
	Key InstanceKey
}

var _ UniqueKeyer = AbsProviderInstance{}

// String returns a string representation of the instance address suitable for
// display in the UI when talking about providers in global scope.
//
// This representation isn't so appropriate for situations when talking about
// provider instances only within a specific module. In that case it might be
// better to translate to a [LocalProviderInstance] and use its string
// representation so that the provider is described using the module's
// chosen short local name, rather than the global provider source address.
func (pi AbsProviderInstance) String() string {
	var buf strings.Builder
	if !pi.Module.IsRoot() {
		buf.WriteString(pi.Module.String())
		buf.WriteByte('.')
	}
	fmt.Fprintf(&buf, "provider[%s]", pi.Provider)
	if pi.Key != nil {
		buf.WriteString(pi.Key.String())
	}
	return buf.String()
}

// UniqueKey implements UniqueKeyer.
func (pi AbsProviderInstance) UniqueKey() UniqueKey {
	return absProviderInstanceKey{pi.String()}
}

type absProviderInstanceKey struct {
	s string
}

// uniqueKeySigil implements UniqueKey.
func (a absProviderInstanceKey) uniqueKeySigil() {}
