// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cliconfig

import (
	"fmt"
	"path/filepath"

	"github.com/hashicorp/hcl"
	hclast "github.com/hashicorp/hcl/hcl/ast"
	hcltoken "github.com/hashicorp/hcl/hcl/token"
	hcl2 "github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/svchost"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/cliconfig/ociauthconfig"
	"github.com/opentofu/opentofu/internal/getproviders"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ProviderInstallation is the structure of the "provider_installation"
// nested block within the CLI configuration.
type ProviderInstallation struct {
	Methods []*ProviderInstallationMethod

	// DevOverrides allows overriding the normal selection process for
	// a particular subset of providers to force using a particular
	// local directory and disregard version numbering altogether.
	// This is here to allow provider developers to conveniently test
	// local builds of their plugins in a development environment, without
	// having to fuss with version constraints, dependency lock files, and
	// so forth.
	//
	// This is _not_ intended for "production" use because it bypasses the
	// usual version selection and checksum verification mechanisms for
	// the providers in question. To make that intent/effect clearer, some
	// OpenTofu commands emit warnings when overrides are present. Local
	// mirror directories are a better way to distribute "released"
	// providers, because they are still subject to version constraints and
	// checksum verification.
	DevOverrides map[addrs.Provider]getproviders.PackageLocalDir
}

// decodeProviderInstallationFromConfig uses the HCL AST API directly to
// decode "provider_installation" blocks from the given file.
//
// This uses the HCL AST directly, rather than HCL's decoder, because the
// intended configuration structure can't be represented using the HCL
// decoder's struct tags. This structure is intended as something that would
// be relatively easier to deal with in HCL 2 once we eventually migrate
// CLI config over to that, and so this function is stricter than HCL 1's
// decoder would be in terms of exactly what configuration shape it is
// expecting.
//
// Note that this function wants the top-level file object which might or
// might not contain provider_installation blocks, not a provider_installation
// block directly itself.
func decodeProviderInstallationFromConfig(hclFile *hclast.File) ([]*ProviderInstallation, tfdiags.Diagnostics) {
	var ret []*ProviderInstallation
	var diags tfdiags.Diagnostics

	root := hclFile.Node.(*hclast.ObjectList)

	// This is a rather odd hybrid: it's a HCL 2-like decode implemented using
	// the HCL 1 AST API. That makes it a bit awkward in places, but it allows
	// us to mimic the strictness of HCL 2 (making a later migration easier)
	// and to support a block structure that the HCL 1 decoder can't represent.
	for _, block := range root.Items {
		if block.Keys[0].Token.Value() != "provider_installation" {
			continue
		}
		// HCL only tracks whether the input was JSON or native syntax inside
		// individual tokens, so we'll use our block type token to decide
		// and assume that the rest of the block must be written in the same
		// syntax, because syntax is a whole-file idea.
		isJSON := block.Keys[0].Token.JSON
		if block.Assign.Line != 0 && !isJSON {
			// Seems to be an attribute rather than a block
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid provider_installation block",
				fmt.Sprintf("The provider_installation block at %s must not be introduced with an equals sign.", block.Pos()),
			))
			continue
		}
		if len(block.Keys) > 1 && !isJSON {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid provider_installation block",
				fmt.Sprintf("The provider_installation block at %s must not have any labels.", block.Pos()),
			))
		}

		pi := &ProviderInstallation{}
		devOverrides := make(map[addrs.Provider]getproviders.PackageLocalDir)

		body, ok := block.Val.(*hclast.ObjectType)
		if !ok {
			// We can't get in here with native HCL syntax because we
			// already checked above that we're using block syntax, but
			// if we're reading JSON then our value could potentially be
			// anything.
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid provider_installation block",
				fmt.Sprintf("The provider_installation block at %s must not be introduced with an equals sign.", block.Pos()),
			))
			continue
		}

		for _, methodBlock := range body.List.Items {
			if methodBlock.Assign.Line != 0 && !isJSON {
				// Seems to be an attribute rather than a block
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Invalid provider_installation method block",
					fmt.Sprintf("The items inside the provider_installation block at %s must all be blocks.", block.Pos()),
				))
				continue
			}
			if len(methodBlock.Keys) > 1 && !isJSON {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Invalid provider_installation method block",
					fmt.Sprintf("The blocks inside the provider_installation block at %s may not have any labels.", block.Pos()),
				))
			}

			methodBody, ok := methodBlock.Val.(*hclast.ObjectType)
			if !ok {
				// We can't get in here with native HCL syntax because we
				// already checked above that we're using block syntax, but
				// if we're reading JSON then our value could potentially be
				// anything.
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Invalid provider_installation method block",
					fmt.Sprintf("The items inside the provider_installation block at %s must all be blocks.", block.Pos()),
				))
				continue
			}

			methodTypeStr := methodBlock.Keys[0].Token.Value().(string)
			var location ProviderInstallationLocation
			var include, exclude []string
			switch methodTypeStr {
			case "direct":
				type BodyContent struct {
					Include []string `hcl:"include"`
					Exclude []string `hcl:"exclude"`
				}
				var bodyContent BodyContent
				err := hcl.DecodeObject(&bodyContent, methodBody)
				if err != nil {
					diags = diags.Append(tfdiags.Sourceless(
						tfdiags.Error,
						"Invalid provider_installation method block",
						fmt.Sprintf("Invalid %s block at %s: %s.", methodTypeStr, block.Pos(), err),
					))
					continue
				}
				location = ProviderInstallationDirect
				include = bodyContent.Include
				exclude = bodyContent.Exclude
			case "filesystem_mirror":
				type BodyContent struct {
					Path    string   `hcl:"path"`
					Include []string `hcl:"include"`
					Exclude []string `hcl:"exclude"`
				}
				var bodyContent BodyContent
				err := hcl.DecodeObject(&bodyContent, methodBody)
				if err != nil {
					diags = diags.Append(tfdiags.Sourceless(
						tfdiags.Error,
						"Invalid provider_installation method block",
						fmt.Sprintf("Invalid %s block at %s: %s.", methodTypeStr, block.Pos(), err),
					))
					continue
				}
				if bodyContent.Path == "" {
					diags = diags.Append(tfdiags.Sourceless(
						tfdiags.Error,
						"Invalid provider_installation method block",
						fmt.Sprintf("Invalid %s block at %s: \"path\" argument is required.", methodTypeStr, block.Pos()),
					))
					continue
				}
				location = ProviderInstallationFilesystemMirror(bodyContent.Path)
				include = bodyContent.Include
				exclude = bodyContent.Exclude
			case "network_mirror":
				type BodyContent struct {
					URL     string   `hcl:"url"`
					Include []string `hcl:"include"`
					Exclude []string `hcl:"exclude"`
				}
				var bodyContent BodyContent
				err := hcl.DecodeObject(&bodyContent, methodBody)
				if err != nil {
					diags = diags.Append(tfdiags.Sourceless(
						tfdiags.Error,
						"Invalid provider_installation method block",
						fmt.Sprintf("Invalid %s block at %s: %s.", methodTypeStr, block.Pos(), err),
					))
					continue
				}
				if bodyContent.URL == "" {
					diags = diags.Append(tfdiags.Sourceless(
						tfdiags.Error,
						"Invalid provider_installation method block",
						fmt.Sprintf("Invalid %s block at %s: \"url\" argument is required.", methodTypeStr, block.Pos()),
					))
					continue
				}
				location = ProviderInstallationNetworkMirror(bodyContent.URL)
				include = bodyContent.Include
				exclude = bodyContent.Exclude
			case "oci_mirror":
				var moreDiags tfdiags.Diagnostics
				location, include, exclude, moreDiags = decodeOCIMirrorInstallationMethodBlock(methodBody)
				diags = diags.Append(moreDiags)
				if moreDiags.HasErrors() {
					continue
				}
			case "dev_overrides":
				if len(pi.Methods) > 0 {
					// We require dev_overrides to appear first if it's present,
					// because dev_overrides effectively bypass the normal
					// selection process for a particular provider altogether,
					// and so they don't participate in the usual
					// include/exclude arguments and priority ordering.
					diags = diags.Append(tfdiags.Sourceless(
						tfdiags.Error,
						"Invalid provider_installation method block",
						fmt.Sprintf("The dev_overrides block at at %s must appear before all other installation methods, because development overrides always have the highest priority.", methodBlock.Pos()),
					))
					continue
				}

				// The content of a dev_overrides block is a mapping from
				// provider source addresses to local filesystem paths. To get
				// our decoding started, we'll use the normal HCL decoder to
				// populate a map of strings and then decode further from
				// that.
				var rawItems map[string]string
				err := hcl.DecodeObject(&rawItems, methodBody)
				if err != nil {
					diags = diags.Append(tfdiags.Sourceless(
						tfdiags.Error,
						"Invalid provider_installation method block",
						fmt.Sprintf("Invalid %s block at %s: %s.", methodTypeStr, block.Pos(), err),
					))
					continue
				}

				for rawAddr, rawPath := range rawItems {
					addr, moreDiags := addrs.ParseProviderSourceString(rawAddr)
					if moreDiags.HasErrors() {
						diags = diags.Append(tfdiags.Sourceless(
							tfdiags.Error,
							"Invalid provider installation dev overrides",
							fmt.Sprintf("The entry %q in %s is not a valid provider source string.\n\n%s", rawAddr, block.Pos(), moreDiags.Err().Error()),
						))
						continue
					}
					dirPath := filepath.Clean(rawPath)
					devOverrides[addr] = getproviders.PackageLocalDir(dirPath)
				}

				continue // We won't add anything to pi.MethodConfigs for this one

			default:
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Invalid provider_installation method block",
					fmt.Sprintf("Unknown provider installation method %q at %s.", methodTypeStr, methodBlock.Pos()),
				))
				continue
			}

			pi.Methods = append(pi.Methods, &ProviderInstallationMethod{
				Location: location,
				Include:  include,
				Exclude:  exclude,
			})
		}

		if len(devOverrides) > 0 {
			pi.DevOverrides = devOverrides
		}

		ret = append(ret, pi)
	}

	return ret, diags
}

// decodeOCIMirrorInstallationMethodBlock decodes the content of an oci_mirror block
// from inside a provider_installation block.
func decodeOCIMirrorInstallationMethodBlock(methodBody *hclast.ObjectType) (location ProviderInstallationLocation, include, exclude []string, diags tfdiags.Diagnostics) {
	type BodyContent struct {
		RepositoryTemplate string   `hcl:"repository_template"`
		Include            []string `hcl:"include"`
		Exclude            []string `hcl:"exclude"`
	}
	var bodyContent BodyContent
	err := hcl.DecodeObject(&bodyContent, methodBody)
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid provider_installation method block",
			fmt.Sprintf("Invalid oci_mirror block at %s: %s.", methodBody.Pos(), err),
		))
		return nil, nil, nil, diags
	}
	if bodyContent.RepositoryTemplate == "" {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid provider_installation method block",
			fmt.Sprintf("Invalid oci_mirror block at %s: \"repository_template\" argument is required.", methodBody.Pos()),
		))
		return nil, nil, nil, diags
	}

	// If the given template is not valid at all then we'd prefer to give immediate
	// feedback on that rather than the user discovering it only once they try to
	// install a provider from this source, so we do as much of the work of
	// parsing and checking the template here as possible. There are still a few
	// situations we cannot detect until we know exactly which provider source
	// address we're trying to map, but our aim is to detect here any situation
	// that would make this template invalid for _any_ given source address.

	// CLI configuration still mainly uses legacy HCL 1, but we'll use HCL 2's
	// template engine for this argument because otherwise we'd need to bring in
	// "HIL", which is another legacy codebase that was historically used as the
	// template engine with HCL 1. HCL 1 generates low-quality source location
	// information, so for now we'll just accept that any diagnostics from this
	// will not include source snippets.
	templateExpr, hclDiags := hclsyntax.ParseTemplate([]byte(bodyContent.RepositoryTemplate), "<oci_mirror repository_template>", hcl2.InitialPos)
	diags = diags.Append(hclDiags)
	if hclDiags.HasErrors() {
		return nil, nil, nil, diags
	}

	// The fact that we use HCL templates for this is not exposed outside of this
	// package, and instead we encapsulate the mapping in a plain callback function.
	// This helper also performs validation of the template, returning error diagnostics
	// if it has any problems that would make it invalid regardless of specific provider
	// source address.
	repoMapping, mappingDiags := prepareOCIMirrorRepositoryMapping(templateExpr, bodyContent.Include, methodBody.Pos())
	diags = diags.Append(mappingDiags)
	if mappingDiags.HasErrors() {
		return nil, nil, nil, diags
	}

	location = ProviderInstallationOCIMirror{
		RepositoryMapping: repoMapping,
	}
	include = bodyContent.Include
	exclude = bodyContent.Exclude
	return location, include, exclude, diags
}

func prepareOCIMirrorRepositoryMapping(templateExpr hclsyntax.Expression, include []string, pos hcltoken.Pos) (func(addrs.Provider) (registryDomain, repositoryName string, err error), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	var templateHasHostname, templateHasNamespace, templateHasType bool
	for _, traversal := range templateExpr.Variables() {
		switch name := traversal.RootName(); name {
		case "hostname":
			templateHasHostname = true
		case "namespace":
			templateHasNamespace = true
		case "type":
			templateHasType = true
		default:
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid oci_mirror repository template",
				fmt.Sprintf(
					"Invalid oci_mirror block at %s: the symbol %q is not available for an OCI mirror repository address template. Only \"hostname\", \"namespace\", and \"type\" are available.",
					pos, name,
				),
			))
			// We continue anyway, because we might be able to collect other errors
			// if the template is invalid in multiple ways.
		}
	}

	// The template must include at least one reference to any source address
	// component that isn't isn't exactly matched by all of the "include" patterns,
	// because otherwise the mapping would be ambiguous.
	includePatterns, err := getproviders.ParseMultiSourceMatchingPatterns(include)
	if err != nil {
		// Invalid patterns get caught later when we finally assemble the provider
		// sources, so we intentionally don't produce an error here to avoid
		// reporting the same problem twice. Instead, we just skip the
		// template checking altogether by returning early.
		return func(p addrs.Provider) (registryDomain string, repositoryName string, err error) {
			// We should not actually get here because overall config validation will
			// detect this problem and report it anyway, but this is here just for
			// robustness in case this accidentally becomes reachable in future.
			return "", "", fmt.Errorf("oci_mirror installation source has invalid 'include' patterns: %w", err)
		}, diags
	}
	hostnames := map[svchost.Hostname]struct{}{}
	namespaces := map[string]struct{}{}
	types := map[string]struct{}{}
	for _, pattern := range includePatterns {
		if pattern.Hostname != svchost.Hostname(getproviders.Wildcard) {
			hostnames[pattern.Hostname] = struct{}{}
		}
		if pattern.Namespace != getproviders.Wildcard {
			namespaces[pattern.Namespace] = struct{}{}
		}
		if pattern.Type != getproviders.Wildcard {
			types[pattern.Type] = struct{}{}
		}
	}
	if len(hostnames) != 1 && !templateHasHostname {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid oci_mirror repository template",
			fmt.Sprintf("Invalid oci_mirror block at %s: template must refer to the \"hostname\" symbol unless the \"include\" argument selects exactly one registry hostname.", pos),
		))
	}
	if len(namespaces) != 1 && !templateHasNamespace {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid oci_mirror repository template",
			fmt.Sprintf("Invalid oci_mirror block at %s: template must refer to the \"namespace\" symbol unless the \"include\" argument selects exactly one provider namespace.", pos),
		))
	}
	if len(types) != 1 && !templateHasType {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid oci_mirror repository template",
			fmt.Sprintf("Invalid oci_mirror block at %s: template must refer to the \"type\" symbol unless the \"include\" argument selects exactly one provider.", pos),
		))
	}
	if diags.HasErrors() {
		return nil, diags
	}

	// The above dealt with some likely problems that we can return tailored diagnoses
	// for. We'll also catch some other potential problems, such as type errors returned
	// by HCL expression operators, by actually trying to evaluate the template. We'll
	// do this in two passes: the first using unknown values of type string so that
	// we can achieve some moderately-high-quality diagnostics for type-related problems,
	// and then the second using known placeholder values that we can use to check whether
	// the resulting syntax seems sensible but for which we can't really generate good
	// error messages because we can't really know why the result turned out to be invalid.
	hclCtx := &hcl2.EvalContext{
		Variables: map[string]cty.Value{
			"hostname":  cty.UnknownVal(cty.String),
			"namespace": cty.UnknownVal(cty.String),
			"type":      cty.UnknownVal(cty.String),
		},
	}
	_, hclDiags := templateExpr.Value(hclCtx) // HCL itself can catch any type-related errors
	diags = diags.Append(hclDiags)
	if hclDiags.HasErrors() {
		// Since these diagnostics are coming from HCLv2 itself, they will
		// describe the source location as "<oci_mirror repository_template>"
		// rather than an actual file location. This is just another
		// unfortunate consequence of continuing to use legacy HCL
		// for the CLI configuration. :(
		return nil, diags
	}
	exampleAddr, exampleDiags := evalOCIProviderMirrorRepositoryTemplate(templateExpr, addrs.Provider{
		Hostname:  svchost.Hostname("example.com"),
		Namespace: "example-namespace",
		Type:      "example-type",
	}, pos)
	diags = diags.Append(exampleDiags)
	if exampleDiags.HasErrors() {
		return nil, diags // This should not catch anything that the previous call didn't, but we'll handle it anyway to make sure
	}
	// If we've got this far without finding a problem then exampleVal
	// should be a string containing some sort of valid OCI repository
	// address, although we can't assume anything about what exactly it
	// refers to, only validate its syntax.
	_, _, err = ociauthconfig.ParseRepositoryAddressPrefix(exampleAddr)
	if err != nil {
		// We can't really say anything specific here because we know nothing
		// about what the author's intention was in writing this template and
		// it would be confusing to reveal the fixed placeholder provider address
		// we used for this test, so we'll keep this generic. Not ideal, but
		// we've put in a bunch of effort above to minimize the chances of
		// reaching this last-resort error message.
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid oci_mirror repository template",
			fmt.Sprintf("Invalid oci_mirror block at %s: template must generate a valid OCI repository address, including a registry hostname followed by a slash and then a repository name.", pos),
		))
		return nil, diags
	}

	// If everything we did above succeeded then we've eliminated many of the
	// ways that the template could be invalid. There are still a few left
	// but we'll need to handle those ones dynamically on request instead.
	return func(p addrs.Provider) (registryDomain string, repositoryName string, err error) {
		repoAddrRaw, diags := evalOCIProviderMirrorRepositoryTemplate(templateExpr, p, pos)
		if diags.HasErrors() {
			// The provider installer returns normal error values rather than full
			// diagnostics, so this function is defined similarly and will do its
			// best to transform diagnostics into reasonable naked errors.
			//
			// Due to the checks we did above before returning this function,
			// it should be very unlikely to return errors here but possible
			// if the user wrote something really weird/complex in the template,
			// such as a conditional expression that only performs an invalid
			// operation when given specific input that doesn't match the
			// example input we tried above.
			return "", "", diags.Err()
		}
		return ociauthconfig.ParseRepositoryAddressPrefix(repoAddrRaw)
	}, diags
}

func evalOCIProviderMirrorRepositoryTemplate(templateExpr hclsyntax.Expression, providerAddr addrs.Provider, pos hcltoken.Pos) (string, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	hclCtx := &hcl2.EvalContext{
		Variables: map[string]cty.Value{
			"hostname":  cty.StringVal(providerAddr.Hostname.String()),
			"namespace": cty.StringVal(providerAddr.Namespace),
			"type":      cty.StringVal(providerAddr.Type),
		},
	}
	val, hclDiags := templateExpr.Value(hclCtx)
	diags = diags.Append(hclDiags)
	if hclDiags.HasErrors() {
		return "", diags
	}
	val, err := convert.Convert(val, cty.String)
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid oci_mirror repository template",
			fmt.Sprintf("Invalid oci_mirror block at %s: template must produce a string value.", pos),
		))
		return "", diags
	}
	if val.IsNull() {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid oci_mirror repository template",
			fmt.Sprintf("Invalid oci_mirror block at %s: template must not produce a null value.", pos),
		))
		return "", diags
	}
	return val.AsString(), diags
}

// ProviderInstallationMethod represents an installation method block inside
// a provider_installation block.
type ProviderInstallationMethod struct {
	Location ProviderInstallationLocation
	Include  []string `hcl:"include"`
	Exclude  []string `hcl:"exclude"`
}

// ProviderInstallationLocation is an interface type representing the
// different installation location types. The concrete implementations of
// this interface are:
//
//   - [ProviderInstallationDirect]:                 install from the provider's origin registry
//   - [ProviderInstallationFilesystemMirror] (dir): install from a local filesystem mirror
//   - [ProviderInstallationNetworkMirror] (host):   install from a network mirror
//   - [ProviderInstallationOCIMirror]:              use OCI registries as if they were a network mirror
type ProviderInstallationLocation interface {
	providerInstallationLocation()
}

type providerInstallationDirect [0]byte

func (i providerInstallationDirect) providerInstallationLocation() {}

// ProviderInstallationDirect is a ProviderInstallationSourceLocation
// representing installation from a provider's origin registry.
var ProviderInstallationDirect ProviderInstallationLocation = providerInstallationDirect{}

func (i providerInstallationDirect) GoString() string {
	return "cliconfig.ProviderInstallationDirect"
}

// ProviderInstallationFilesystemMirror is a ProviderInstallationSourceLocation
// representing installation from a particular local filesystem mirror. The
// string value is the filesystem path to the mirror directory.
type ProviderInstallationFilesystemMirror string

func (i ProviderInstallationFilesystemMirror) providerInstallationLocation() {}

func (i ProviderInstallationFilesystemMirror) GoString() string {
	return fmt.Sprintf("cliconfig.ProviderInstallationFilesystemMirror(%q)", i)
}

// ProviderInstallationNetworkMirror is a ProviderInstallationSourceLocation
// representing installation from a particular local network mirror. The
// string value is the HTTP base URL exactly as written in the configuration,
// without any normalization.
type ProviderInstallationNetworkMirror string

func (i ProviderInstallationNetworkMirror) providerInstallationLocation() {}

func (i ProviderInstallationNetworkMirror) GoString() string {
	return fmt.Sprintf("cliconfig.ProviderInstallationNetworkMirror(%q)", i)
}

// ProviderInstallationOCIMirror is a ProviderInstallationSourceLocation
// representing a rule for using repositories in OCI registries as a
// provider network mirror.
//
// This is conceptualy similar to [ProviderInstallationNetworkMirror], but
// this on uses the OCI Distribution protocol instead of the OpenTofu-specific
// Provider Mirror Protocol.
type ProviderInstallationOCIMirror struct {
	// RepositoryMapping is a function that translates from an OpenTofu-style
	// logical provider source address to a physical OCI repository address.
	//
	// For a valid OCI mirror source this function encapsulates the details
	// of evaluating the user-provided mapping template from the configuration,
	// so that callers of this function don't need to be aware of the
	// implementation detail that this uses HCL templates.
	RepositoryMapping func(addrs.Provider) (registryDomain, repositoryName string, err error)
}

func (i ProviderInstallationOCIMirror) providerInstallationLocation() {}

func (i ProviderInstallationOCIMirror) GoString() string {
	// There isn't really any useful string representation of the content
	// of this type, but this is only used for internal stuff like describing
	// mismatches in tests, so just naming the type is good enough for now.
	return "cliconfig.ProviderInstallationNetworkMirror{/*...*/}"
}
