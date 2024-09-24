package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/e2e"
)

func requiredProviderFixture(provider string) *e2e.Fixture {
	return e2e.NewFixture().File("required_providers.tofu", fmt.Sprintf(`
                terraform {
                    required_providers {
                        %s
                    }
                }
                `, provider))
}

func TestConfig_RequiredProvidersFailures(t *testing.T) {
	failures := []struct {
		name     string
		provider string
		summary  string
		detail   string
	}{{
		name:     `blocks not allowed`,
		provider: `block_not_allowed {}`,
		summary:  `Unexpected "block_not_allowed" block`,
		detail:   `Blocks are not allowed here.`,
	}, {
		name:     `invalid legacy name`,
		provider: `das--hes = "1.0.0"`,
		summary:  `Invalid provider name`,
		detail:   `cannot use multiple consecutive dashes`,
	}, {
		name:     `invalid legacy version`,
		provider: `version = "> alpha is a version"`,
		summary:  `Invalid version constraint`,
		detail:   `This string does not use correct version constraint syntax.`,
	}, {
		name:     "bad value",
		provider: `bad-value = [4]`,
		summary:  "Invalid required_providers object",
		detail:   "required_providers entries must be strings or objects.",
	}, {
		name: "bad case",
		provider: `Local = {
                        source = "opentofu/local"
                }`,
		summary: "Invalid provider local name",
		detail:  "Provider names must be normalized. Replace \"Local\" with \"local\" to fix this error.",
	}, {
		// TODO This scenario does not have a good error message
		name: "bad key",
		provider: `bad-key = {
                        5 = "key"
                }`,
		summary: "Invalid Attribute",
		detail:  "Invalid attribute value for provider requirement: cty.NumberIntVal(5)",
	}, {
		name: "bad version string",
		provider: `bad-version-string = {
                        version = 27
                }`,
		summary: "Invalid version constraint",
		detail:  "Version must be specified as a string.",
	}, {
		name: "bad version value",
		provider: `bad-version-value = {
                        version = "> not a version"
                }`,
		summary: "Invalid version constraint",
		detail:  "This string does not use correct version constraint syntax.",
	}, {
		name: "bad source string",
		provider: `invalid-source-string = {
                        source = 5
                }`,
		summary: "Invalid source",
		detail:  "Source must be specified as a string.",
	}, {
		name: "no evaluation",
		provider: `local = {
                        source = var.provider_source
                }`,
		summary: "Invalid source",
		detail:  "Source must be specified as a string.",
	}, {
		name: "bad source structure",
		provider: `invalid-souce-structure = {
                        source = "foo/bar/baz/bitsy"
                }`,
		summary: "Invalid provider source string",
		detail:  "The \"source\" attribute must be in the format \"[hostname/][namespace/]name\"",
	}, {
		name: "legacy upgrade",
		provider: `local = {
                        source = "-/local"
                }`,
		summary: "Invalid legacy provider address",
		detail:  "This configuration or its associated state refers to the unqualified provider \"local\".\n\nYou must complete the Terraform 0.13 upgrade process before upgrading to later versions.",
	}, {
		name:     "empty provider",
		provider: `lo--cal = {}`,
		summary:  "Invalid provider local name",
		detail:   "lo--cal is an invalid provider local name: cannot use multiple consecutive dashes",
	}}

	for _, tc := range failures {
		tc := tc
		t.Run("failure "+tc.name, func(t *testing.T) {
			t.Parallel()
			f := requiredProviderFixture(tc.provider)

			e2e.NewScenario(f, t).Tofu("init", "-json").Run().
				Failure().Stdout().JSON().
				HasDiagnostic(tc.detail, func(d e2e.Diagnostic) bool {
					return d.Severity == "error" && d.Summary == tc.summary && d.Detail == tc.detail
				})
		})
	}
}

func TestConfig_RequiredProvidersRoot(t *testing.T) {
	// This intentionally does not create or manage lock files and tests a clean slate.

	/*
	   # TODO: potential legacy bugfix
	   # Interestingly for this case, it appears that we allow uppercase aliases in required_providers, but they are non-functional
	   # through the rest of the application.  We should normalize them or error in this case.
	*/
	t.Run("duplicate legacy provider", func(t *testing.T) {
		f := requiredProviderFixture(`
            local = "= 2.2.3"
            // Duplicate that normalizes to "local" using legacy logic
            Local = "2.2.3"
        `).RequireNetwork()
		e2e.NewScenario(f, t).Tofu("init", "-json").Run().
			Success().Stdout().JSON().
			// TODO Map order is not preserved in the diagnostic message and cannot be compared
			HasDiagnostic("duplicate legacy provider", func(d e2e.Diagnostic) bool {
				return d.Severity == "warning" && d.Summary == "Duplicate required provider"
			})
	})

	t.Run("undecorated provider", func(t *testing.T) {
		f := requiredProviderFixture(`local = {
                        source = "local"
                }`).RequireNetwork()
		e2e.NewScenario(f, t).Tofu("init").Run().
			Success().Stdout().After("Installing hashicorp/local").Contains("Installed hashicorp/local")
	})

	t.Run("decorated provider", func(t *testing.T) {
		f := requiredProviderFixture(`local = {
                        source = "opentofu/local"
                }`).RequireNetwork()
		e2e.NewScenario(f, t).Tofu("init").Run().
			Success().Stdout().After("Installing opentofu/local").Contains("Installed opentofu/local")
	})

	t.Run("fully qualified provider", func(t *testing.T) {
		t.Parallel()
		f := requiredProviderFixture(`local = {
                        source = "registry.opentofu.org/opentofu/local"
                }`).RequireNetwork()
		e2e.NewScenario(f, t).Tofu("init").Run().
			Success().Stdout().After("Installing opentofu/local").Contains("Installed opentofu/local")
	})

	t.Run("opentofu/hashcorp potential issue", func(t *testing.T) {
		f := requiredProviderFixture(`
                ot = {
                        source = "opentofu/local"
                }
                hc = {
                        source = "local" # defaults to hashcorp/local
                }`).RequireNetwork()
		output := e2e.NewScenario(f, t).Tofu("init", "-json").Run().
			Success().Stdout()
		output.After("Installing opentofu/local").Contains("Installed opentofu/local")
		output.After("Installing hashicorp/local").Contains("Installed hashicorp/local")
		// TODO Map order is not preserved in the diagnostic message and cannot be compared
		output.JSON().HasDiagnostic("multiple provider warning", func(d e2e.Diagnostic) bool {
			return d.Severity == "warning" && d.Summary == "Potential provider misconfiguration" && strings.HasPrefix(d.Detail, "OpenTofu has detected multiple providers of type local")
		})
	})
}

// TODO test provider configs
// TODO test implicit providers
// TODO test providers in sub-modules (w/config)
