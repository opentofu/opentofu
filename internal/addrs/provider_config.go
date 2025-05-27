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
// LocalProviderInstance or AbsProviderInstance, in order to represent situations
// where a value might either be module-local or absolute but the decision
// cannot be made until runtime.
//
// Where possible, use either LocalProviderInstance or AbsProviderInstance directly
// instead, to make intent more clear. ProviderInstance can be used only in
// situations where the recipient of the value has some out-of-band way to
// determine a "current module" to use if the value turns out to be
// a LocalProviderInstance.
//
// Recipients of non-nil ProviderInstance values that actually need
// AbsProviderInstance values should call ResolveAbsProviderAddr on the
// *configs.Config value representing the root module configuration, which
// handles the translation from local to fully-qualified using mapping tables
// defined in the configuration.
//
// Recipients of a ProviderInstance value can assume it can contain only a
// LocalProviderInstance value, an AbsProviderInstanceValue, or nil to represent
// the absence of a provider config in situations where that is meaningful.
type ProviderInstance interface {
	providerInstance()
}

// LocalProviderInstance is the address of a provider configuration from the
// perspective of references in a particular module.
//
// Finding the corresponding AbsProviderInstance will require looking up the
// LocalName in the providers table in the module's configuration; there is
// no syntax-only translation between these types.
type LocalProviderInstance struct {
	LocalName string

	// If not empty, Alias identifies which non-default (aliased) provider
	// configuration this address refers to.
	Alias string
}

var _ ProviderInstance = LocalProviderInstance{}

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

// StringCompact is an alternative to String that returns the form that can
// be parsed by ParseProviderInstanceCompact, without the "provider." prefix.
func (pc LocalProviderInstance) StringCompact() string {
	if pc.Alias != "" {
		return fmt.Sprintf("%s.%s", pc.LocalName, pc.Alias)
	}
	return pc.LocalName
}

// AbsProviderInstance is the absolute address of a provider instance
// within a particular module instance.
type AbsProviderInstance struct {
	Module   Module
	Provider Provider
	Alias    string
}

var _ ProviderInstance = AbsProviderInstance{}

// ParseAbsProviderInstance parses the given traversal as an absolute provider
// instance address. The following are examples of traversals that can be
// successfully parsed as absolute provider instance addresses:
//
//   - provider["registry.opentofu.org/hashicorp/aws"]
//   - provider["registry.opentofu.org/hashicorp/aws"].foo
//   - module.bar.provider["registry.opentofu.org/hashicorp/aws"]
//   - module.bar.module.baz.provider["registry.opentofu.org/hashicorp/aws"].foo
//
// This type of address is used, for example, to record the relationships
// between resources and provider instances in the state structure.
// This type of address is typically not used prominently in the UI, except in
// error messages that refer to provider instances.
func ParseAbsProviderInstance(traversal hcl.Traversal) (AbsProviderInstance, tfdiags.Diagnostics) {
	pc, key, diags := ParseKeyedAbsProviderInstance(traversal)
	if key != NoKey {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid provider configuration address",
			Detail:   "A provider address must not include an instance key.",
			Subject:  traversal.SourceRange().Ptr(),
		})
	}
	return pc, diags
}

// ParseKeyedAbsProviderInstance behaves identically to ParseAbsProviderInstance, but additionally
// allows an instance key after the alias.
func ParseKeyedAbsProviderInstance(traversal hcl.Traversal) (AbsProviderInstance, InstanceKey, tfdiags.Diagnostics) {
	modInst, remain, diags := parseModuleInstancePrefix(traversal)
	var ret AbsProviderInstance
	var key InstanceKey

	// Providers cannot resolve within module instances, so verify that there
	// are no instance keys in the module path before converting to a Module.
	for _, step := range modInst {
		if step.InstanceKey != NoKey {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid provider configuration address",
				Detail:   "A provider configuration must not appear in a module instance that uses count or for_each.",
				Subject:  remain.SourceRange().Ptr(),
			})
			return ret, key, diags
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
		return ret, key, diags
	}
	if len(remain) > 4 {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid provider configuration address",
			Detail:   "Extraneous operators after provider configuration reference.",
			Subject:  remain[4:].SourceRange().Ptr(),
		})
		return ret, key, diags
	}

	if tt, ok := remain[1].(hcl.TraverseIndex); ok {
		if !tt.Key.Type().Equals(cty.String) {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid provider configuration address",
				Detail:   "The prefix \"provider.\" must be followed by a provider type name.",
				Subject:  remain[1].SourceRange().Ptr(),
			})
			return ret, key, diags
		}
		p, sourceDiags := ParseProviderSourceString(tt.Key.AsString())
		ret.Provider = p
		if sourceDiags.HasErrors() {
			diags = diags.Append(sourceDiags)
			return ret, key, diags
		}
	} else {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid provider configuration address",
			Detail:   "The prefix \"provider.\" must be followed by a provider type name.",
			Subject:  remain[1].SourceRange().Ptr(),
		})
		return ret, key, diags
	}

	if len(remain) > 2 {
		if tt, ok := remain[2].(hcl.TraverseAttr); ok {
			ret.Alias = tt.Name
		} else {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid provider configuration address",
				Detail:   "Provider type name must be followed by a configuration alias name.",
				Subject:  remain[2].SourceRange().Ptr(),
			})
			return ret, key, diags
		}
	}

	if len(remain) > 3 {
		if tt, ok := remain[3].(hcl.TraverseIndex); ok {
			var keyErr error
			key, keyErr = ParseInstanceKey(tt.Key)
			if keyErr != nil {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid provider configuration address",
					Detail:   fmt.Sprintf("Invalid provider instance key: %s.", keyErr.Error()),
					Subject:  remain[3].SourceRange().Ptr(),
				})
			}
		} else {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid provider configuration address",
				Detail:   "A provider configuration alias can be followed only by an instance key in brackets.",
				Subject:  remain[3].SourceRange().Ptr(),
			})
			return ret, key, diags
		}
	}

	return ret, key, diags
}

// ParseAbsProviderInstanceStr is a helper wrapper around ParseAbsProviderInstance
// that takes a string and parses it with the HCL native syntax traversal parser
// before interpreting it.
//
// This should be used only in specialized situations since it will cause the
// created references to not have any meaningful source location information.
// If a reference string is coming from a source that should be identified in
// error messages then the caller should instead parse it directly using a
// suitable function from the HCL API and pass the traversal itself to
// ParseAbsProviderInstance.
//
// Error diagnostics are returned if either the parsing fails or the analysis
// of the traversal fails. There is no way for the caller to distinguish the
// two kinds of diagnostics programmatically. If error diagnostics are returned
// the returned address is invalid.
func ParseAbsProviderInstanceStr(str string) (AbsProviderInstance, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	traversal, parseDiags := hclsyntax.ParseTraversalAbs([]byte(str), "", hcl.Pos{Line: 1, Column: 1})
	diags = diags.Append(parseDiags)
	if parseDiags.HasErrors() {
		return AbsProviderInstance{}, diags
	}
	addr, addrDiags := ParseAbsProviderInstance(traversal)
	diags = diags.Append(addrDiags)
	return addr, diags
}
func ParseKeyedAbsProviderInstanceStr(str string) (AbsProviderInstance, InstanceKey, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	traversal, parseDiags := hclsyntax.ParseTraversalAbs([]byte(str), "", hcl.Pos{Line: 1, Column: 1})
	diags = diags.Append(parseDiags)
	if parseDiags.HasErrors() {
		return AbsProviderInstance{}, nil, diags
	}
	addr, key, addrDiags := ParseKeyedAbsProviderInstance(traversal)
	diags = diags.Append(addrDiags)
	return addr, key, diags
}

func ParseLegacyAbsProviderInstanceStr(str string) (AbsProviderInstance, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	traversal, parseDiags := hclsyntax.ParseTraversalAbs([]byte(str), "", hcl.Pos{Line: 1, Column: 1})
	diags = diags.Append(parseDiags)
	if parseDiags.HasErrors() {
		return AbsProviderInstance{}, diags
	}

	addr, addrDiags := ParseLegacyAbsProviderInstance(traversal)
	diags = diags.Append(addrDiags)
	return addr, diags
}

// ParseLegacyAbsProviderInstance parses the given traversal as an absolute
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
func ParseLegacyAbsProviderInstance(traversal hcl.Traversal) (AbsProviderInstance, tfdiags.Diagnostics) {
	modInst, remain, diags := parseModuleInstancePrefix(traversal)
	var ret AbsProviderInstance

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

// ProviderInstanceDefault returns the address of the default provider config of
// the given type inside the receiving module instance.
func (m ModuleInstance) ProviderInstanceDefault(provider Provider) AbsProviderInstance {
	return AbsProviderInstance{
		Module:   m.Module(),
		Provider: provider,
	}
}

// ProviderInstanceAliased returns the address of an aliased provider config of
// the given type and alias inside the receiving module instance.
func (m ModuleInstance) ProviderInstanceAliased(provider Provider, alias string) AbsProviderInstance {
	return AbsProviderInstance{
		Module:   m.Module(),
		Provider: provider,
		Alias:    alias,
	}
}

// providerInstance Implements addrs.ProviderInstance.
func (pc AbsProviderInstance) providerInstance() {}

// Inherited returns an address that the receiving configuration address might
// inherit from in a parent module. The second bool return value indicates if
// such inheritance is possible, and thus whether the returned address is valid.
//
// Inheritance is possible only for default (un-aliased) providers in modules
// other than the root module. Even if a valid address is returned, inheritance
// may not be performed for other reasons, such as if the calling module
// provided explicit provider configurations within the call for this module.
// The ProviderTransformer graph transform in the main tofu module has the
// authoritative logic for provider inheritance, and this method is here mainly
// just for its benefit.
func (pc AbsProviderInstance) Inherited() (AbsProviderInstance, bool) {
	// Can't inherit if we're already in the root.
	if len(pc.Module) == 0 {
		return AbsProviderInstance{}, false
	}

	// Can't inherit if we have an alias.
	if pc.Alias != "" {
		return AbsProviderInstance{}, false
	}

	// Otherwise, we might inherit from a configuration with the same
	// provider type in the parent module instance.
	parentMod := pc.Module.Parent()
	return AbsProviderInstance{
		Module:   parentMod,
		Provider: pc.Provider,
	}, true

}

// LegacyString() returns a legacy-style AbsProviderInstance string and should only be used for legacy state shimming.
func (pc AbsProviderInstance) LegacyString() string {
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

// String() returns a string representation of an AbsProviderInstance in a format like the following examples:
//
//   - provider["example.com/namespace/name"]
//   - provider["example.com/namespace/name"].alias
//   - module.module-name.provider["example.com/namespace/name"]
//   - module.module-name.provider["example.com/namespace/name"].alias
func (pc AbsProviderInstance) String() string {
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

func (pc AbsProviderInstance) InstanceString(key InstanceKey) string {
	if key == NoKey {
		return pc.String()
	}
	return pc.String() + key.String()
}
