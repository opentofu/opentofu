// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cliconfig

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl"
	hclast "github.com/hashicorp/hcl/hcl/ast"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

// OCIDefaultCredentials corresponds to one oci_default_credentials block in
// the CLI configuration.
//
// This represents just one part of the overall OCI credentials policy, and so needs
// to be considered in conjunction with all of the OCICredentials objects across
// the CLI configuration too.
type OCIDefaultCredentials struct {
	// DiscoverAmbientCredentials decides whether OpenTofu will attempt to find
	// credentials "ambiently" in the environment where OpenTofu is running, such
	// as searching the conventional locations for Docker-style configuration files.
	//
	// This defaults to true, but operators can set it to false to completely opt out
	// of OpenTofu using credentials from anywhere other than elsewhere in the
	// OpenTofu CLI configuration.
	DiscoverAmbientCredentials bool

	// DockerStyleConfigFiles forces a specific set of filenames to try to use as
	// sources of OCI credentials, interpreting them as Docker CLI-style configuration
	// files.
	//
	// If this is nil, OpenTofu uses a default set of search locations mimicking the
	// behavior of other tools in the ecosystem such as Podman, Buildah, etc.
	//
	// If this is non-nil but zero length, it effectively disables using any Docker CLI-style
	// configuration files at all, but if DiscoverAmbientCredentials is also true then
	// future versions of OpenTofu might try to use other sources of ambient credentials.
	//
	// This field is always nil if DiscoverAmbientCredentials is false, because this field
	// exists only to customize one aspect of the "ambient credentials" discovery behavior.
	DockerStyleConfigFiles []string

	// The name of a Docker-style credential helper program to use for any domain
	// that doesn't have its own specific credential helper configured.
	//
	// If this is not set then a default credential helper might still be discovered
	// from the ambient credentials sources, unless such discovery is disabled using
	// the other fields in this struct.
	DefaultDockerCredentialHelper string
}

// newDefaultOCIDefaultCredentials returns an [OCIDefaultCredentials] object representing
// the default settings used when no oci_default_credentials blocks are present.
//
// Each call to this function returns a distinct object, so it's safe for the caller
// to modify the result to reflect any customizations.
func newDefaultOCIDefaultCredentials() *OCIDefaultCredentials {
	return &OCIDefaultCredentials{
		DiscoverAmbientCredentials:    true,
		DockerStyleConfigFiles:        nil,
		DefaultDockerCredentialHelper: "",
	}
}

// decodeOCIDefaultCredentialsFromConfig uses the HCL AST API directly to
// decode "oci_default_credentials" blocks from the given file.
//
// The overall CLI configuration is only allowed to contain one
// oci_default_credentials block, but the caller deals with that constraint
// separately after searching all of the CLI configuration files.
//
// This uses the HCL AST directly, rather than HCL's decoder, to continue
// our precedent of trying to constrain new features only to what could be
// supported compatibly in a hypothetical future HCL 2-based implementation
// of the CLI configuration language.
//
// Note that this function wants the top-level file object which might or
// might not contain oci_default_credentials blocks, not an oci_default_credentials
// block directly itself.
func decodeOCIDefaultCredentialsFromConfig(hclFile *hclast.File, filename string) ([]*OCIDefaultCredentials, tfdiags.Diagnostics) {
	var ret []*OCIDefaultCredentials
	var diags tfdiags.Diagnostics

	root, ok := hclFile.Node.(*hclast.ObjectList)
	if !ok {
		// A HCL file that doesn't have an object list at its root is weird, but
		// dealing with that is outside the scope of this function.
		// (In practice both the native syntax and JSON parsers for HCL force
		// the root to be an ObjectList, so we should not get here for any real file.)
		return ret, diags
	}
	for _, block := range root.Items {
		if block.Keys[0].Token.Value() != "oci_default_credentials" {
			continue
		}

		// HCL only tracks whether the input was JSON or native syntax inside
		// individual tokens, so we'll use our block type token to decide
		// and assume that the rest of the block must be written in the same
		// syntax, because syntax is a whole-file idea.
		const errInvalidSummary = "Invalid oci_default_credentials block"
		isJSON := block.Keys[0].Token.JSON
		if block.Assign.Line != 0 && !isJSON {
			// Seems to be an attribute rather than a block
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				errInvalidSummary,
				fmt.Sprintf("The oci_default_credentials block at %s must not be introduced with an equals sign.", block.Pos()),
			))
			continue
		}
		if len(block.Keys) > 1 && !isJSON {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				errInvalidSummary,
				fmt.Sprintf("The oci_default_credentials block at %s must not have any labels.", block.Pos()),
			))
			continue
		}
		body, ok := block.Val.(*hclast.ObjectType)
		if !ok {
			// We can't get in here with native HCL syntax because we
			// already checked above that we're using block syntax, but
			// if we're reading JSON then our value could potentially be
			// anything.
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				errInvalidSummary,
				fmt.Sprintf("The oci_default_credentials block at %s must be represented by a JSON object.", block.Pos()),
			))
			continue
		}

		result, blockDiags := decodeOCIDefaultCredentialsBlockBody(body, filename)
		diags = diags.Append(blockDiags)
		if result != nil {
			ret = append(ret, result)
		}
	}

	return ret, diags
}

func decodeOCIDefaultCredentialsBlockBody(body *hclast.ObjectType, filename string) (*OCIDefaultCredentials, tfdiags.Diagnostics) {
	const errInvalidSummary = "Invalid oci_default_credentials block"
	var diags tfdiags.Diagnostics

	// Any relative file paths in this block are resolved relative to the directory
	// containing the file where this block came from.
	baseDir := filepath.Dir(filename)

	// Although decodeOCIDefaultCredentialsFromConfig did some lower-level decoding
	// to try to force HCL 2-compatible syntax, the _content_ of this block is all
	// just relatively-simple arguments and so we can use HCL 1's decoder here.
	type BodyContent struct {
		DiscoverAmbientCredentials     *bool     `hcl:"discover_ambient_credentials"`
		DockerStyleConfigFiles         *[]string `hcl:"docker_style_config_files"`
		DefaultDockerCredentialsHelper *string   `hcl:"docker_credentials_helper"`
	}
	var bodyContent BodyContent
	err := hcl.DecodeObject(&bodyContent, body)
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			errInvalidSummary,
			fmt.Sprintf("Invalid oci_default_credentials block at %s: %s.", body.Pos(), err),
		))
		return nil, diags
	}

	// We'll start with the default values and then override based on what was
	// specified in the configuration block.
	ret := newDefaultOCIDefaultCredentials()
	if bodyContent.DiscoverAmbientCredentials != nil {
		ret.DiscoverAmbientCredentials = *bodyContent.DiscoverAmbientCredentials
	}
	if bodyContent.DockerStyleConfigFiles != nil {
		// NOTE: non-nil but zero length represents explicitly nothing, rather that the default locations
		ret.DockerStyleConfigFiles = make([]string, len(*bodyContent.DockerStyleConfigFiles))
		for i, configPath := range *bodyContent.DockerStyleConfigFiles {
			if !filepath.IsAbs(configPath) {
				configPath = filepath.Join(baseDir, configPath)
			}
			// We'll also make a best effort to "absolute-ize" the path
			// so that it won't get reinterpreted differently if the process
			// switches to a different working directory after loading the
			// CLI config (as happens with the -chdir global option). In
			// the unlikely event that this fails we'll just accept it
			// because we'll report any inaccessible files in a better way later.
			if absPath, err := filepath.Abs(configPath); err == nil {
				configPath = absPath
			}
			ret.DockerStyleConfigFiles[i] = configPath
		}
	}
	if bodyContent.DefaultDockerCredentialsHelper != nil {
		ret.DefaultDockerCredentialHelper = *bodyContent.DefaultDockerCredentialsHelper
		if !validDockerCredentialHelperName(ret.DefaultDockerCredentialHelper) {
			diags = append(diags, tfdiags.Sourceless(
				tfdiags.Error,
				errInvalidSummary,
				fmt.Sprintf(
					"The oci_default_credentials block at %s specifies the invalid Docker credential helper name %q. Must be a non-empty string that could be used as part of an executable filename.",
					body.Pos(), ret.DefaultDockerCredentialHelper,
				),
			))
		}
	}

	if !ret.DiscoverAmbientCredentials && ret.DockerStyleConfigFiles != nil {
		// docker_style_config_files is a modifier for the discover_ambient_credentials
		// behavior, so can't be used if discovery is totally disabled.
		diags = append(diags, tfdiags.Sourceless(
			tfdiags.Error,
			errInvalidSummary,
			fmt.Sprintf(
				"The oci_default_credentials block at %s disables discovery of ambient credentials, but also sets docker_style_config_files which is relevant only when ambient credentials discovery is enabled.",
				body.Pos(),
			),
		))
	}

	return ret, diags
}

// OCIRepositoryCredentials corresponds directly to a single oci_credentials block
// in the CLI configuration, decoded in isolation. It represents the credentials
// configuration for a set of OCI repositories with a specific registry domain and
// optional repository path prefix.
//
// This represents just one part of the overall OCI credentials policy, and so needs
// to be considered in conjunction with all of the other OCICredentials objects across
// the CLI configuration, and the OCIDefaultCredentials object too.
type OCIRepositoryCredentials struct {
	// RegistryDomain is the domain name (and optional port number) of the registry
	// containing the repositories that these credentials apply to.
	RegistryDomain string

	// RepositoryPathPrefix is an optional path prefix that constrains which
	// repositories on RegistryDomain these credentials can be used for.
	RepositoryPathPrefix string

	// Username and Password are credentials to use for a "Basic"-style
	// authentication method. These are mutually-exclusive with AccessToken
	// and RefreshToken.
	Username, Password string

	// AccessToken and RefreshToken are credentials for an OAuth-style
	// authentication method. These are mutually-exclusive with Username
	// and Password.
	AccessToken, RefreshToken string
}

// TODO: Implement decodeOCICredentialsFromConfig, returning []*OCIRepositoryCredentials

func validDockerCredentialHelperName(n string) bool {
	switch {
	case n == "":
		// It definitely can't be an empty string.
		return false
	case strings.Contains(filepath.ToSlash(n), `/`):
		// The exact details of what's valid here seem OS-specific and so we'll defer
		// the most detailed validation until we know we're actually going to try to
		// run the credentials helper, but at this point we do at least know that
		// the given name is going to be used as part of the filename of an executable
		// and so it definitely can't contain path separators accepted by the current
		// platform.
		return false
	default:
		return true
	}
}
