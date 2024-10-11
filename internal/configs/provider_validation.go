// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"

	"github.com/opentofu/opentofu/internal/addrs"
)

// validateProviderConfigsForTests performs the same role as
// validateProviderConfigs except it validates the providers configured within
// test files.
//
// To do this is calls out to validateProviderConfigs for each run block that
// has ConfigUnderTest set.
//
// In addition, for each run block that executes against the main config it
// validates the providers the run block wants to use match the providers
// specified in the main configuration. It does this without reaching out to
// validateProviderConfigs because the main configuration has already been
// validated, and we don't want to redo all the work that happens in that
// function. So, we only validate the providers our test files define match
// the providers required by the main configuration.
//
// This function does some fairly controversial conversions into structures
// expected by validateProviderConfigs but since we're just using it for
// validation we'll still get the correct error messages, and we can make the
// declaration ranges line up sensibly so we'll even get good diagnostics.
func validateProviderConfigsForTests(cfg *Config) (diags hcl.Diagnostics) {
	// FIXME: Make dynamic-expanded providers work with tests
	if len(cfg.Module.Tests) != 0 {
		panic("validateProviderConfigsForTests not yet updated for dynamic-expanded providers")
	} else {
		return nil
	}

	/*
		for name, test := range cfg.Module.Tests {
			for _, run := range test.Runs {

				if run.ConfigUnderTest == nil {
					// Then we're calling out to the main configuration under test.
					//
					// We just need to make sure that the providers we are setting
					// actually match the providers in the configuration. The main
					// configuration has already been validated, so we don't need to
					// do the whole thing again.

					if len(run.Providers) > 0 {
						// This is the easy case, we can just validate that the
						// provider types match.
						for _, provider := range run.Providers {

							parentType, childType := provider.InParent.providerType, provider.InChild.providerType
							if parentType.IsZero() {
								parentType = addrs.NewDefaultProvider(provider.InParent.Name)
							}
							if childType.IsZero() {
								childType = addrs.NewDefaultProvider(provider.InChild.Name)
							}

							if !childType.Equals(parentType) {
								diags = append(diags, &hcl.Diagnostic{
									Severity: hcl.DiagError,
									Summary:  "Provider type mismatch",
									Detail: fmt.Sprintf(
										"The local name %q in %s represents provider %q, but %q in the root module represents %q.\n\nThis means the provider definition for %q within %s, or other provider definitions with the same name, have been referenced by multiple run blocks and assigned to different provider types.",
										provider.InParent.Name, name, parentType, provider.InChild.Name, childType, provider.InParent.Name, name),
									Subject: provider.InParent.NameRange.Ptr(),
								})
							}
						}

						// Skip to the next file, we only need to verify the types
						// specified here.
						continue
					}

					// Otherwise, we need to verify that the providers required by
					// the configuration match the types defined by our test file.

					for _, requirement := range cfg.Module.ProviderRequirements.RequiredProviders {
						if provider, exists := test.Providers[requirement.Name]; exists {

							providerType := provider.providerType
							if providerType.IsZero() {
								providerType = addrs.NewDefaultProvider(provider.Name)
							}

							if !providerType.Equals(requirement.Type) {
								diags = append(diags, &hcl.Diagnostic{
									Severity: hcl.DiagError,
									Summary:  "Provider type mismatch",
									Detail: fmt.Sprintf(
										"The provider %q in %s represents provider %q, but %q in the root module represents %q.\n\nThis means the provider definition for %q within %s, or other provider definitions with the same name, have been referenced by multiple run blocks and assigned to different provider types.",
										provider.Addr().StringCompact(), name, providerType, requirement.Name, requirement.Type, provider.Addr().StringCompact(), name),
									Subject: provider.DeclRange.Ptr(),
								})
							}
						}

						for _, alias := range requirement.Aliases {
							if provider, exists := test.Providers[alias.StringCompact()]; exists {

								providerType := provider.providerType
								if providerType.IsZero() {
									providerType = addrs.NewDefaultProvider(provider.Name)
								}

								if !providerType.Equals(requirement.Type) {
									diags = append(diags, &hcl.Diagnostic{
										Severity: hcl.DiagError,
										Summary:  "Provider type mismatch",
										Detail: fmt.Sprintf(
											"The provider %q in %s represents provider %q, but %q in the root module represents %q.\n\nThis means the provider definition for %q within %s, or other provider definitions with the same name, have been referenced by multiple run blocks and assigned to different provider types.",
											provider.Addr().StringCompact(), name, providerType, alias.StringCompact(), requirement.Type, provider.Addr().StringCompact(), name),
										Subject: provider.DeclRange.Ptr(),
									})
								}
							}
						}
					}

					for _, provider := range cfg.Module.ProviderConfigs {

						providerType := provider.providerType
						if providerType.IsZero() {
							providerType = addrs.NewDefaultProvider(provider.Name)
						}

						if testProvider, exists := test.Providers[provider.Addr().StringCompact()]; exists {
							testProviderType := testProvider.providerType
							if testProviderType.IsZero() {
								testProviderType = addrs.NewDefaultProvider(testProvider.Name)
							}

							if !providerType.Equals(testProviderType) {
								diags = append(diags, &hcl.Diagnostic{
									Severity: hcl.DiagError,
									Summary:  "Provider type mismatch",
									Detail: fmt.Sprintf(
										"The provider %q in %s represents provider %q, but %q in the root module represents %q.\n\nThis means the provider definition for %q within %s has been referenced by multiple run blocks and assigned to different provider types.",
										testProvider.Addr().StringCompact(), name, testProviderType, provider.Addr().StringCompact(), providerType, testProvider.Addr().StringCompact(), name),
									Subject: testProvider.DeclRange.Ptr(),
								})
							}
						}
					}
				} else {
					// Then we're executing another module. We'll just call out to
					// validateProviderConfigs and let it do the whole thing.

					providers := run.Providers
					if len(providers) == 0 {
						// If the test run didn't provide us a subset of providers
						// to use, we'll build our own. This is so that we can fit
						// into the schema expected by validateProviderConfigs.

						matchedProviders := make(map[string]PassedProviderConfig)

						// We'll go over all the requirements in the module first
						// and see if we have defined any providers for that
						// requirement. If we have, then we'll take not of that.

						for _, requirement := range cfg.Module.ProviderRequirements.RequiredProviders {

							if provider, exists := test.Providers[requirement.Name]; exists {
								matchedProviders[requirement.Name] = PassedProviderConfig{
									InChild: &ProviderConfigRef{
										Name:         requirement.Name,
										NameRange:    requirement.DeclRange,
										providerType: requirement.Type,
									},
									InParent: &ProviderConfigRef{
										Name:         provider.Name,
										NameRange:    provider.NameRange,
										Alias:        provider.Alias,
										providerType: provider.providerType,
									},
								}
							}

							// Also, remember to check for any aliases the module
							// expects.

							for _, alias := range requirement.Aliases {
								key := alias.StringCompact()

								if provider, exists := test.Providers[key]; exists {
									matchedProviders[key] = PassedProviderConfig{
										InChild: &ProviderConfigRef{
											Name:         requirement.Name,
											NameRange:    requirement.DeclRange,
											Alias:        alias.Alias,
											AliasRange:   requirement.DeclRange.Ptr(),
											providerType: requirement.Type,
										},
										InParent: &ProviderConfigRef{
											Name:         provider.Name,
											NameRange:    provider.NameRange,
											Alias:        provider.Alias,
											providerType: provider.providerType,
										},
									}
								}

							}

						}

						// Next, we'll look at any providers the module has defined
						// directly. If we have an equivalent provider in the test
						// file then we'll add that in to override it. If the module
						// has both built a required providers block and a provider
						// block for the same provider, we'll overwrite the one we
						// made for the requirement provider. We get more precise
						// DeclRange objects from provider blocks so it makes for
						// better error messages to use these.

						for _, provider := range cfg.Module.ProviderConfigs {
							key := provider.Addr().StringCompact()

							if testProvider, exists := test.Providers[key]; exists {
								matchedProviders[key] = PassedProviderConfig{
									InChild: &ProviderConfigRef{
										Name:         provider.Name,
										NameRange:    provider.DeclRange,
										Alias:        provider.Alias,
										AliasRange:   provider.DeclRange.Ptr(),
										providerType: provider.providerType,
									},
									InParent: &ProviderConfigRef{
										Name:         testProvider.Name,
										NameRange:    testProvider.NameRange,
										Alias:        testProvider.Alias,
										providerType: testProvider.providerType,
									},
								}
							}
						}

						// Last thing to do here is add them into the actual
						// providers list that is going into the module call below.
						for _, provider := range matchedProviders {
							providers = append(providers, provider)
						}

					}

					// Let's make a little fake module call that we can use to call
					// into validateProviderConfigs.
					mc := &ModuleCall{
						Name:      run.Name,
						Providers: providers,
						DeclRange: run.Module.DeclRange,
					}

					diags = append(diags, validateProviderConfigs(mc, run.ConfigUnderTest, nil)...)
				}
			}
		}

		return diags
	*/
}

// validateProviderConfigs walks the full configuration tree from the root
// module outward, static validation rules to the various combinations of
// provider configuration, required_providers values, and module call providers
// mappings.
//
// To retain compatibility with previous terraform versions, empty "proxy
// provider blocks" are still allowed within modules, though they will
// generate warnings when the configuration is loaded. The new validation
// however will generate an error if a suitable provider configuration is not
// passed in through the module call.
//
// In earlier versions we had fully-resolved provider instance addresses
// statically in the configuration, but we now support dynamic alias and for_each
// for providers and so the instances are not decided until runtime and so the
// main language runtime's "validate" walk is responsible for checking those,
// while this focuses only on whether each provider block can be associated with
// a provider.
func validateProviderConfigs(parentCall *ModuleCall, cfg *Config) (diags hcl.Diagnostics) {
	mod := cfg.Module

	for name, child := range cfg.Children {
		mc := mod.ModuleCalls[name]
		diags = append(diags, validateProviderConfigs(mc, child)...)
	}

	// These are the explicitly-declared or implied local names in this
	// module's provider local name namespace, mapped to the global
	// source address they each correspond to.
	localNames := map[string]addrs.Provider{}

	if mod.ProviderRequirements != nil {
		// Track all known local types too to ensure we don't have duplicated
		// with different local names.
		localTypes := map[string]bool{}

		// check for duplicate requirements of the same type
		for _, req := range mod.ProviderRequirements.RequiredProviders {
			if localTypes[req.Type.String()] {
				// find the last declaration to give a better error
				prevDecl := ""
				for localName, typ := range localNames {
					if typ.Equals(req.Type) {
						prevDecl = localName
					}
				}

				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagWarning,
					Summary:  "Duplicate required provider",
					Detail: fmt.Sprintf(
						"Provider %s with the local name %q was previously required as %q. A provider can only be required once within required_providers.",
						req.Type.ForDisplay(), req.Name, prevDecl,
					),
					Subject: &req.DeclRange,
				})
			} else if addrs.IsDefaultProvider(req.Type) {
				// Now check for possible implied duplicates, where a provider
				// block uses a default namespaced provider, but that provider
				// was required via a different name.
				impliedLocalName := req.Type.Type
				// We have to search through the configs for a match, since the keys contains any aliases.
				for _, pc := range mod.ProviderConfigs {
					if pc.Name == impliedLocalName && req.Name != impliedLocalName {
						diags = append(diags, &hcl.Diagnostic{
							Severity: hcl.DiagWarning,
							Summary:  "Duplicate required provider",
							Detail: fmt.Sprintf(
								"Provider %s with the local name %q was implicitly required via a configuration block as %q. The provider configuration block name must match the name used in required_providers.",
								req.Type.ForDisplay(), req.Name, req.Type.Type,
							),
							Subject: &req.DeclRange,
						})
						break
					}
				}
			}

			localTypes[req.Type.String()] = true
		}
	}

	checkImpliedProviderNames := func(resourceConfigs map[string]*Resource) {
		// Now that we have all the provider configs and requirements validated,
		// check for any resources which use an implied localname which doesn't
		// match that of required_providers
		for _, r := range resourceConfigs {
			// We're looking for resources with no specific provider reference
			if r.ProviderConfigRef != nil {
				continue
			}

			localName := r.Addr().ImpliedProvider()

			_, err := addrs.ParseProviderPart(localName)
			if err != nil {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid provider local name",
					Detail:   fmt.Sprintf("%q is an invalid implied provider local name: %s", localName, err),
					Subject:  r.DeclRange.Ptr(),
				})
				continue
			}

			if _, ok := localNames[localName]; ok {
				// OK, this was listed directly in the required_providers
				continue
			}

			defAddr := addrs.ImpliedProviderForUnqualifiedType(localName)

			// Now make sure we don't have the same provider required under a
			// different name.
			for prevLocalName, addr := range localNames {
				if addr.Equals(defAddr) {
					diags = append(diags, &hcl.Diagnostic{
						Severity: hcl.DiagWarning,
						Summary:  "Duplicate required provider",
						Detail: fmt.Sprintf(
							"Provider %q was implicitly required via resource %q, but listed in required_providers as %q. Either the local name in required_providers must match the resource name, or the %q provider must be assigned within the resource block.",
							defAddr, r.Addr(), prevLocalName, prevLocalName,
						),
						Subject: &r.DeclRange,
					})
				}
			}
		}
	}
	checkImpliedProviderNames(mod.ManagedResources)
	checkImpliedProviderNames(mod.DataResources)

	if cfg.Path.IsRoot() {
		// nothing else to do in the root module
		return diags
	}

	// FIXME: Figure out what else still makes sense to do here, rather
	// than in the language runtime.
	diags = append(diags, &hcl.Diagnostic{
		Severity: hcl.DiagWarning,
		Summary:  "The provider reference validation code in the configs package is not yet fully updated for dynamic resolution",
	})

	return diags
}

func providerName(name, alias string) string {
	if alias != "" {
		name = name + "." + alias
	}
	return name
}
