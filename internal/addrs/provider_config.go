// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"fmt"
	"strings"

	"github.com/opentofu/opentofu/internal/tfdiags"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// LocalOrAbsProvider is an interface type whose dynamic type can be either
// LocalProvider or AbsProvider, in order to represent situations where a
// value might either be module-local or absolute but the decision cannot be
// made until runtime.
//
// Where possible, use either LocalProvider or AbsProvider directly instead,
// to make intent more clear. LocalOrAbsProvider can be used only
// in situations where the recipient of the value has some out-of-band way to
// determine a "current module" to use if the value turns out to be
// a LocalProvider.
//
// Recipients of non-nil LocalOrAbsProvider values that actually need
// AbsProvider values should call ResolveAbsProviderAddr on the
// *configs.Config value representing the root module configuration, which
// handles the translation from local to fully-qualified using mapping tables
// defined in the configuration.
//
// Recipients of a LocalOrAbsProvider value can assume it can contain only a
// LocalProvider value, an AbsProvider value, or nil to represent the absence
// of a provider config in situations where that is meaningful.
type LocalOrAbsProvider interface {
	localOrAbsProvider()
}

var _ LocalOrAbsProvider = LocalProvider{}
var _ LocalOrAbsProvider = AbsProvider{}

// LocalProvider is the address of a particular provider configuration from
// the perspective of references in a particular module.
//
// This uses the "LocalName" idea, which is a module-local lookup table from
// short names (chosen by the module author) to fully-qualified [Provider]
// addresses. Therefore finding the corresponding [AbsProviderInstance]
// requires consulting that lookup table; there is no syntax-only translation
// between these types.
//
// This address variant is for references to "provider" blocks in the
// configuration, regardless of how many instances they might have. To represent
// a reference to a specific instance of a provider that was declared in a
// provider configuration (e.g. using for_each), use [LocalProviderInstance]
// instead to capture the instance key.
type LocalProvider struct {
	LocalName string
	Alias     string
}

var _ Referenceable = LocalProvider{}

// NewDefaultLocalProviderInstance returns the address of the default (un-aliased)
// configuration for the provider with the given local type name.
func NewDefaultLocalProvider(localName string) LocalProvider {
	return LocalProvider{
		LocalName: localName,
	}
}

// Instance returns the address of the instance of the provider that
// has the given instance key.
func (pc LocalProvider) Instance(key InstanceKey) LocalProviderInstance {
	return LocalProviderInstance{
		LocalName: pc.LocalName,
		Alias:     pc.Alias,
		Key:       key,
	}
}

func (pc LocalProvider) String() string {
	if pc.LocalName == "" {
		// Should never happen; always indicates a bug
		return "provider.<invalid>"
	}
	if pc.Alias != "" {
		return fmt.Sprintf("provider.%s.%s", pc.LocalName, pc.Alias)
	}
	return "provider." + pc.LocalName
}

// UniqueKey implements UniqueKeyer and Referenceable.
func (pc LocalProvider) UniqueKey() UniqueKey {
	// A LocalProvider can be its own UniqueKey
	return pc
}

// uniqueKeySigil implements UniqueKey.
func (pc LocalProvider) uniqueKeySigil() {}

// referenceableSigil implements Referenceable.
func (pc LocalProvider) referenceableSigil() {}

// localOrAbsProvider Implements addrs.LocalOrAbsProvider
func (pc LocalProvider) localOrAbsProvider() {}

// LocalProviderInstance is the address of a provider configuration from the
// perspective of references in a particular module.
//
// Finding the corresponding ConfigProviderInstance will require looking up the
// LocalName in the providers table in the module's configuration; there is
// no syntax-only translation between these types.
type LocalProviderInstance struct {
	LocalName string
	Alias     string

	// Key MUST be [NoKey] unless Alias != "", because default provider
	// configurations are not allowed to have dynamic instances.
	Key InstanceKey
}

// LocalProvider returns the address of the provider configuration that this instance
// belongs to.
func (pc LocalProviderInstance) LocalProvider() LocalProvider {
	return LocalProvider{
		LocalName: pc.LocalName,
		Alias:     pc.Alias,
	}
}

func (pc LocalProviderInstance) String() string {
	if pc.LocalName == "" {
		// Should never happen; always indicates a bug
		return "provider.<invalid>"
	}

	if pc.Alias != "" {
		if pc.Key != NoKey {
			return fmt.Sprintf("provider.%s.%s%s", pc.LocalName, pc.Alias, pc.Key)
		}
		return fmt.Sprintf("provider.%s.%s", pc.LocalName, pc.Alias)
	} else if pc.Key != NoKey {
		// Should never happen; always indicates a bug
		return fmt.Sprintf("provider.%s.<invalid>%s", pc.LocalName, pc.Key)
	}

	return "provider." + pc.LocalName
}

// StringCompact is an alternative to String that returns the compact form
// without the "provider." prefix.
func (pc LocalProviderInstance) StringCompact() string {
	if pc.LocalName == "" {
		// Should never happen; always indicates a bug
		return "<invalid>"
	}

	if pc.Alias != "" {
		if pc.Key != NoKey {
			return fmt.Sprintf("%s.%s%s", pc.LocalName, pc.Alias, pc.Key)
		}
		return fmt.Sprintf("%s.%s", pc.LocalName, pc.Alias)
	} else if pc.Key != NoKey {
		// Should never happen; always indicates a bug
		return fmt.Sprintf("%s.<invalid>%s", pc.LocalName, pc.Key)
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

// ConfigProvider represents an unexpanded "provider" block in the
// configuration.
//
// A provider block can expand to zero or more [AbsProviderInstance]
// addresses at runtime, if it either has for_each set or is declared
// inside a module with for_each set.
type ConfigProvider struct {
	Module   Module
	Provider Provider

	// Alias is empty for a default ("unaliased") provider configuration,
	// or non-empty for an additional ("aliased") provider configuration.
	//
	// Only additional provider configurations can have multiple dynamic
	// instances generated using for_each.
	Alias string
}

func (p ConfigProvider) String() string {
	// The string representation of this type is only for debug/error message
	// use -- it never needs to be stored anywhere -- so we'll just borrow
	// the AbsProvider implementation as a good-enough placeholder.
	placeholder := AbsProvider{
		Module:   p.Module.UnkeyedInstanceShim(),
		Provider: p.Provider,
		Alias:    p.Alias,
	}
	return placeholder.String()
}

// AbsProvider represents an unexpanded "provider" configuration block within
// an already-expanded module instance.
//
// This is the absolute analog to [LocalProvider], used only for connecting
// resource instances recorded in the prior state with the provider block
// that declares their associated provider instances during dependency graph
// construction. For most other situations [AbsProviderInstance] is the better
// type to use, because it also captures one specific instance dynamically
// declared by a provider configuration block.
type AbsProvider struct {
	Module   ModuleInstance
	Provider Provider
	Alias    string
}

// Instance returns the address of the instance of this provider that has the
// given instance key.
func (p AbsProvider) Instance(key InstanceKey) AbsProviderInstance {
	return AbsProviderInstance{
		Module:   p.Module,
		Provider: p.Provider,
		Alias:    p.Alias,
		Key:      key,
	}
}

func (p AbsProvider) String() string {
	// Since AbsProvider is a relatively-narrow-scoped address type, and
	// doesn't really strictly _need_ a string representation, we'll just
	// borrow the syntax for AbsProviderInstance with no key so that
	// we've got a useful representation to use in debug output.
	return p.Instance(NoKey).String()
}

// localOrAbsProvider implements LocalOrAbsProvider.
func (pi AbsProvider) localOrAbsProvider() {}

// AbsProviderInstance represents the fully-qualified of an instance of a
// provider, after instance expansion is complete.
//
// Each "provider" block in the configuration can become zero or more
// AbsProviderInstance after expansion, and all of the expanded instances
// must have unique AbsProviderInstance addresses.
type AbsProviderInstance struct {
	Module   ModuleInstance
	Provider Provider
	Alias    string

	// Key is the additional instance key used to distinguish multiple
	// provider instances generated from the same provider block.
	//
	// This is always [NoKey] when Alias is empty, because default
	// provider configurations are required to be singletons.
	//
	// When Alias is non-empty, [NoKey] indicates a single-instance
	// additional provider configuration, while a [StringKey] value
	// indicates one instance of a dynamic-expanded additional provider
	// configuration, using the for_each argument.
	//
	// Currently there is no support for "count" in provider configurations
	// and so this can currently never be an [IntKey] value, but callers
	// should try to keep this generic (similar treatment as for [InstanceKey]
	// values in other address types) to keep our options open for the future.
	Key InstanceKey
}

var _ UniqueKeyer = AbsProviderInstance{}

// ParseAbsProviderInstance parses the given traversal as an absolute
// provider instance address. The following are examples of traversals that can be
// successfully parsed as provider instance addresses using this function:
//
//   - provider["registry.opentofu.org/hashicorp/aws"]
//   - provider["registry.opentofu.org/hashicorp/aws"].foo
//   - module.bar.provider["registry.opentofu.org/hashicorp/aws"]
//   - module.bar["foo"].module.baz.provider["registry.opentofu.org/hashicorp/aws"].foo
//
// This type of address is used, for example, to record the relationships
// between resources and provider configurations in the state structure.
// This type of address is typically not used prominently in the UI, except in
// error messages that refer to provider configurations.
func ParseAbsProviderInstance(traversal hcl.Traversal) (AbsProviderInstance, tfdiags.Diagnostics) {
	modInst, remain, diags := parseModuleInstancePrefix(traversal)
	var ret AbsProviderInstance

	ret.Module = modInst

	if len(remain) < 2 || remain.RootName() != "provider" {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid provider configuration address",
			Detail:   "Provider address must begin with \"provider.\", followed by a provider type name.",
			Subject:  remain.SourceRange().Ptr(),
		})
		return ret, diags
	}
	if len(remain) > 4 {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid provider configuration address",
			Detail:   "Extraneous operators after provider configuration address.",
			Subject:  hcl.Traversal(remain[4:]).SourceRange().Ptr(),
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

	if len(remain) > 2 {
		// The next portion specifies the "alias" for an additional provider
		// configuration. If (and only if) this is present, there can
		// optionally be one more step after this specifying an instance
		// key from a multi-instance provider configuration.
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

	if len(remain) > 3 {
		// The final step, if present, selects a dynamically-generated
		// instance of a non-default provider configuration.
		if tt, ok := remain[3].(hcl.TraverseIndex); ok {
			key, err := ParseInstanceKey(tt.Key)
			if err != nil {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid provider configuration address",
					Detail:   fmt.Sprintf("Invalid provider configuration instance key: %s.", tfdiags.FormatError(err)),
					Subject:  remain[3].SourceRange().Ptr(),
				})
				return ret, diags
			}
			ret.Key = key
		} else {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid provider configuration address",
				Detail:   "The final optional step must be an index operator specifying one of the dynamic instances of this provider configuration.",
				Subject:  remain[3].SourceRange().Ptr(),
			})
			return ret, diags
		}
	}

	return ret, diags
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

// AbsProvider returns the address of the provider configuration block that
// would've declared this provider instance.
func (pi AbsProviderInstance) AbsProvider() AbsProvider {
	return AbsProvider{
		Module:   pi.Module,
		Provider: pi.Provider,
		Alias:    pi.Alias,
	}
}

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
	if pi.Alias != "" {
		buf.WriteByte('.')
		buf.WriteString(pi.Alias)
	}
	if pi.Key != NoKey {
		buf.WriteString(pi.Key.String())
	}
	return buf.String()
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

	// OpenTofu v0.12 and earlier didn't have dynamic module instances yet,
	// so if we encounter those then this can't possibly be a legacy address.
	for _, step := range modInst {
		if step.InstanceKey != NoKey {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid provider configuration address",
				Detail:   "Legacy provider instance address cannot contain module instance key",
				Subject:  remain.SourceRange().Ptr(),
			})
			return ret, diags
		}
	}
	ret.Module = modInst

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
		// NOTE: The "legacy" syntax intentionally does not support instance keys
		// because OpenTofu v0.12 and earlier didn't have that feature yet, and
		// so we cannot possibly encounter an old state snapshot using the legacy
		// syntax with an instance key.
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
			ret.Key = StringKey(tt.Name)
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

// UniqueKey implements UniqueKeyer.
func (pi AbsProviderInstance) UniqueKey() UniqueKey {
	return absProviderInstanceKey{pi.String()}
}

type absProviderInstanceKey struct {
	s string
}

// uniqueKeySigil implements UniqueKey.
func (a absProviderInstanceKey) uniqueKeySigil() {}
