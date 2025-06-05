// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/zclconf/go-cty/cty"
)

type PreconfiguredProvider struct {
	Name            string
	Addr            string
	ProtocolVersion int
	Type            addrs.Provider
	DeclRange       hcl.Range
	Aliases         []addrs.LocalProviderConfig
}

type PreconfiguredProviders struct {
	PreconfiguredProviders map[addrs.Provider]*PreconfiguredProvider
	DeclRange              hcl.Range
}

func decodePreconfiguredProvider(block *hcl.Block) (*PreconfiguredProviders, hcl.Diagnostics) {
	ret := &PreconfiguredProviders{
		PreconfiguredProviders: make(map[addrs.Provider]*PreconfiguredProvider),
		DeclRange:              block.DefRange,
	}

	attrs, diags := block.Body.JustAttributes()
	if diags.HasErrors() {
		// Returns an empty RequiredProvider to allow further validations to work properly,
		// allowing to return all the diagnostics correctly.
		return ret, diags
	}

	for name, attr := range attrs {
		rp := &PreconfiguredProvider{
			Name:      name,
			DeclRange: attr.Expr.Range(),
		}
		provAddr, addrsDiags := addrs.ParseProviderSourceString(name)
		if addrsDiags.HasErrors() {
			for _, diagnostic := range addrsDiags.ToHCL() {
				diags = diags.Append(diagnostic)
			}
			return nil, diags
		}

		// Look for a single static string, in case we have the legacy version-only
		// format in the configuration.
		if expr, err := attr.Expr.Value(nil); err == nil && expr.Type().IsPrimitiveType() {
			pType, err := addrs.ParseProviderPart(rp.Name)
			if err != nil {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid provider name",
					Detail:   err.Error(),
					Subject:  attr.Expr.Range().Ptr(),
				})
				continue
			}
			rp.Type = addrs.ImpliedProviderForUnqualifiedType(pType)
			ret.PreconfiguredProviders[provAddr] = rp

			continue
		}

		// verify that the local name is already localized or produce an error.
		nameDiags := checkProviderNameNormalized(name, attr.Expr.Range())
		if nameDiags.HasErrors() {
			diags = append(diags, nameDiags...)
			continue
		}

		kvs, mapDiags := hcl.ExprMap(attr.Expr)
		if mapDiags.HasErrors() {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid required_providers object",
				Detail:   "required_providers entries must be strings or objects.",
				Subject:  attr.Expr.Range().Ptr(),
			})
			continue
		}

	LOOP:
		for _, kv := range kvs {
			key, keyDiags := kv.Key.Value(nil)
			if keyDiags.HasErrors() {
				diags = append(diags, keyDiags...)
				continue
			}

			if key.Type() != cty.String {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid Attribute",
					Detail:   fmt.Sprintf("Invalid attribute value for provider requirement: %#v", key),
					Subject:  kv.Key.Range().Ptr(),
				})
				continue
			}

			switch key.AsString() {
			case "source":
				source, err := kv.Value.Value(nil)
				if err != nil || !source.Type().Equals(cty.String) {
					diags = append(diags, &hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Invalid source",
						Detail:   "Source must be specified as a string.",
						Subject:  kv.Value.Range().Ptr(),
					})
					continue
				}

				fqn, sourceDiags := addrs.ParseProviderSourceString(source.AsString())
				if sourceDiags.HasErrors() {
					hclDiags := sourceDiags.ToHCL()
					// The diagnostics from ParseProviderSourceString don't contain
					// source location information because it has no context to compute
					// them from, and so we'll add those in quickly here before we
					// return.
					for _, diag := range hclDiags {
						if diag.Subject == nil {
							diag.Subject = kv.Value.Range().Ptr()
						}
					}
					diags = append(diags, hclDiags...)
					continue
				}

				provAddr = fqn
				rp.Type = fqn

			case "addr":
				addr, err := kv.Value.Value(nil)
				if err != nil || !addr.Type().Equals(cty.String) {
					diags = append(diags, &hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Invalid addr",
						Detail:   "Addr must be specified as a string.",
						Subject:  kv.Value.Range().Ptr(),
					})
					continue
				}
				rp.Addr = addr.AsString()
			case "protocol_version":
				vers, err := kv.Value.Value(nil)
				if err != nil || !vers.Type().Equals(cty.Number) {
					diags = append(diags, &hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Invalid protocol_version",
						Detail:   "protocol_version must be specified as a number.",
						Subject:  kv.Value.Range().Ptr(),
					})
					continue
				}
				versF := vers.AsBigFloat()
				if !versF.IsInt() {
					diags = append(diags, &hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Invalid protocol_version",
						Detail:   "protocol_version must be specified as an integer.",
						Subject:  kv.Value.Range().Ptr(),
					})
					continue
				}
				versI, _ := versF.Int64()
				rp.ProtocolVersion = int(versI)

			case "configuration_aliases":
				exprs, listDiags := hcl.ExprList(kv.Value)
				if listDiags.HasErrors() {
					diags = append(diags, listDiags...)
					continue
				}

				for _, expr := range exprs {
					traversal, travDiags := hcl.AbsTraversalForExpr(expr)
					if travDiags.HasErrors() {
						diags = append(diags, travDiags...)
						continue
					}

					addr, cfgDiags := ParseProviderConfigCompact(traversal)
					if cfgDiags.HasErrors() {
						diags = append(diags, &hcl.Diagnostic{
							Severity: hcl.DiagError,
							Summary:  "Invalid configuration_aliases value",
							Detail:   `Configuration aliases can only contain references to local provider configuration names in the format of provider.alias`,
							Subject:  kv.Value.Range().Ptr(),
						})
						continue
					}

					if addr.LocalName != name {
						diags = append(diags, &hcl.Diagnostic{
							Severity: hcl.DiagError,
							Summary:  "Invalid configuration_aliases value",
							Detail:   fmt.Sprintf(`Configuration aliases must be prefixed with the provider name. Expected %q, but found %q.`, name, addr.LocalName),
							Subject:  kv.Value.Range().Ptr(),
						})
						continue
					}

					rp.Aliases = append(rp.Aliases, addr)
				}

			default:
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid required_providers object",
					Detail:   `required_providers objects can only contain "version", "source" and "configuration_aliases" attributes. To configure a provider, use a "provider" block.`,
					Subject:  kv.Key.Range().Ptr(),
				})
				break LOOP
			}

		}

		if diags.HasErrors() {
			continue
		}

		// We can add the required provider when there are no errors.
		// If a source was not given, create an implied type.
		if rp.Type.IsZero() {
			pType, err := addrs.ParseProviderPart(rp.Name)
			if err != nil {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid provider name",
					Detail:   err.Error(),
					Subject:  attr.Expr.Range().Ptr(),
				})
			} else {
				rp.Type = addrs.ImpliedProviderForUnqualifiedType(pType)
			}
		}

		ret.PreconfiguredProviders[provAddr] = rp
	}

	return ret, diags
}
