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

	"github.com/opentofu/opentofu/internal/command/cliconfig/ociauthconfig"
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
	// A repository address prefix, in the form "domain/path", that describes which
	// repositories these credentials are to be used for.
	//
	// This string is treated in the same way as the properties of the "auths" object
	// in a container engine configuration file.
	RepositoryPrefix string

	// Username and Password are credentials to use for a "Basic"-style
	// authentication method. These are mutually-exclusive with AccessToken
	// and RefreshToken.
	Username, Password string

	// AccessToken and RefreshToken are credentials for an OAuth-style
	// authentication method. These are mutually-exclusive with Username
	// and Password.
	AccessToken, RefreshToken string

	// DockerCredentialsHelper is the name of a Docker-style credential helper program
	// to use.
	//
	// Docker-style config only allows credential helpers to be configured at
	// whole-registry-domain granularity, so for consistency we only allow this to be
	// set when RepositoryPathPrefix isn't set.
	DockerCredentialHelper string
}

// decodeOCIRepositoryCredentialsFromConfig uses the HCL AST API directly
// to decode "oci_credentials" blocks from the given file.
//
// The overall CLI configuration can contain zero or more blocks of this
// type. We require that each one describes a distinct OCI repository
// address prefix, but that constraint must be enforced by the caller of
// this function because it must be checked across all of the CLI
// configuration files together, rather than just one file at a time.
//
// This uses the HCL AST directly, rather than HCL's decoder, to continue
// our precedent of trying to constrain new features only to what could be
// supported compatibly in a hypothetical future HCL 2-based implementation
// of the CLI configuration language.
//
// Note that this function wants the top-level file object which might or
// might not contain oci_credentials blocks, not an oci_credentials block
// directly itself.
func decodeOCIRepositoryCredentialsFromConfig(hclFile *hclast.File) ([]*OCIRepositoryCredentials, tfdiags.Diagnostics) {
	var ret []*OCIRepositoryCredentials
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
		const errInvalidSummary = "Invalid oci_credentials block"
		if block.Keys[0].Token.Value() != "oci_credentials" {
			continue
		}

		// This helper function compensates for HCL 1's inability to automatically
		// resolve the block label vs. block argument ambiguity in its JSON syntax.
		// (This is why HCL 2 requires explicit schema!)
		const TWO = 2 // To quiet the "mnd" linter
		unwrapHCLObjectKeysFromJSON(block, TWO)
		if len(block.Keys) != TWO {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				errInvalidSummary,
				fmt.Sprintf("The oci_credentials block at %s must have one label, giving an OCI repository address prefix.", block.Pos()),
			))
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
				errInvalidSummary,
				fmt.Sprintf("The oci_credentials block at %s must not be introduced with an equals sign.", block.Pos()),
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
				fmt.Sprintf("The oci_credentials block at %s must be represented by a JSON object.", block.Pos()),
			))
			continue
		}
		label, ok := block.Keys[1].Token.Value().(string)
		if !ok {
			// HCL grammar doesn't allow anything other than string in the key position,
			// so we should not get here.
			panic(fmt.Sprintf("HCL returned non-string label %#v for oci_credentials block", block.Keys[1].Token))
		}

		result, blockDiags := decodeOCICredentialsBlockBody(label, body)
		diags = diags.Append(blockDiags)
		if result != nil {
			ret = append(ret, result)
		}
	}

	return ret, diags
}

func decodeOCICredentialsBlockBody(label string, body *hclast.ObjectType) (*OCIRepositoryCredentials, tfdiags.Diagnostics) {
	const errInvalidSummary = "Invalid oci_credentials block"
	var diags tfdiags.Diagnostics

	// We only validate here, since the repository-matching function in ociauthconfig
	// wants the unparsed string and performs its own parsing step for consistency
	// with the handling of container-engine-style config files.
	_, repositoryPath, labelErr := ociauthconfig.ParseRepositoryAddressPrefix(label)
	if labelErr != nil {
		diags = append(diags, tfdiags.Sourceless(
			tfdiags.Error,
			errInvalidSummary,
			fmt.Sprintf(
				"The oci_credentials block at %s has an invalid block label: %s.",
				body.Pos(), labelErr,
			),
		))
		return nil, diags
	}
	repositoryPrefix := label

	// Although decodeOCICredentialsFromConfig did some lower-level decoding
	// to try to force HCL 2-compatible syntax, the _content_ of this block is all
	// just relatively-simple arguments and so we can use HCL 1's decoder here.
	type BodyContent struct {
		// The following three groups of arguments are mutually-exclusive.

		// Basic-auth-style credentials, statically configured
		Username *string `hcl:"username"`
		Password *string `hcl:"password"`

		// OAuth style credentials
		AccessToken  *string `hcl:"access_token"`
		RefreshToken *string `hcl:"refresh_token"`

		// Docker-style credentials helper providing Basic-auth-style credentials
		// indirectly through an external program
		DockerCredentialsHelper *string `hcl:"docker_credentials_helper"`
	}
	var bodyContent BodyContent
	err := hcl.DecodeObject(&bodyContent, body)
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			errInvalidSummary,
			fmt.Sprintf("Invalid oci_credentials block at %s: %s.", body.Pos(), err),
		))
		return nil, diags
	}

	staticBasicAuth := bodyContent.Username != nil || bodyContent.Password != nil
	oauth := bodyContent.AccessToken != nil || bodyContent.RefreshToken != nil
	dockerCredHelper := bodyContent.DockerCredentialsHelper != nil
	stylesConfigured := trueCount(staticBasicAuth, oauth, dockerCredHelper)
	if stylesConfigured == 0 {
		diags = append(diags, tfdiags.Sourceless(
			tfdiags.Error,
			errInvalidSummary,
			fmt.Sprintf(
				"The oci_credentials block at %s must set either username+password, access_token+refresh_token, or docker_credentials_helper.",
				body.Pos(),
			),
		))
		return nil, diags
	}
	if stylesConfigured > 1 {
		diags = append(diags, tfdiags.Sourceless(
			tfdiags.Error,
			errInvalidSummary,
			fmt.Sprintf(
				"The oci_credentials block at %s must set only one group out of username+password, access_token+refresh_token, or docker_credentials_helper.",
				body.Pos(),
			),
		))
		return nil, diags
	}

	ret := &OCIRepositoryCredentials{
		RepositoryPrefix: repositoryPrefix,
	}
	switch {
	case staticBasicAuth:
		if bodyContent.Username == nil || bodyContent.Password == nil {
			diags = append(diags, tfdiags.Sourceless(
				tfdiags.Error,
				errInvalidSummary,
				fmt.Sprintf(
					"The oci_credentials block at %s must set both username and password together when using static credentials.",
					body.Pos(),
				),
			))
			return nil, diags
		}
		ret.Username = *bodyContent.Username
		ret.Password = *bodyContent.Password
	case oauth:
		// FIXME: Is refresh_roken actually required? We could potentially allow setting
		// only access_token and let the request just immediately fail if the token has expired.
		if bodyContent.AccessToken == nil || bodyContent.RefreshToken == nil {
			diags = append(diags, tfdiags.Sourceless(
				tfdiags.Error,
				errInvalidSummary,
				fmt.Sprintf(
					"The oci_credentials block at %s must set both access_token and refresh_token together when using OAuth-style credentials.",
					body.Pos(),
				),
			))
			return nil, diags
		}
		ret.AccessToken = *bodyContent.AccessToken
		ret.RefreshToken = *bodyContent.RefreshToken
	case dockerCredHelper:
		ret.DockerCredentialHelper = *bodyContent.DockerCredentialsHelper
		if repositoryPath != "" {
			diags = append(diags, tfdiags.Sourceless(
				tfdiags.Error,
				errInvalidSummary,
				fmt.Sprintf(
					"The oci_credentials block at %s cannot set docker_credentials_helper with a repository path: credential helpers only support credentials for whole domains.",
					body.Pos(),
				),
			))
		}
		if !validDockerCredentialHelperName(ret.DockerCredentialHelper) {
			diags = append(diags, tfdiags.Sourceless(
				tfdiags.Error,
				errInvalidSummary,
				fmt.Sprintf(
					"The oci_credentials block at %s specifies the invalid Docker credential helper name %q. Must be a non-empty string that could be used as part of an executable filename.",
					body.Pos(), ret.DockerCredentialHelper,
				),
			))
		}
		if diags.HasErrors() {
			return nil, diags
		}
	}

	return ret, diags
}

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

func trueCount(flags ...bool) int {
	ret := 0
	for _, flag := range flags {
		if flag {
			ret++
		}
	}
	return ret
}

// unwrapHCLObjectKeysFromJSON cleans up an edge case that can occur when
// parsing JSON as input using the old HCL 1 parser: if we're parsing JSON
// then directly nested items will show up as additional "keys".
//
// For objects that expect a fixed number of keys, this breaks the
// decoding process. This function unwraps the object into what it would've
// looked like if it came directly from HCL 1 by specifying the number of keys
// you expect.
//
// Example:
//
// { "foo": { "baz": {} } }
//
// Will show up with Keys being: []string{"foo", "baz"}
// when we really just want the first one. This function will fix this.
//
// This function is a fun old helper cribbed from a much older version before
// HCL 2, where the main language was also implemented using HCL 1:
// https://github.com/opentofu/opentofu/blob/e0fd3ddd704b230897723a7ca251f36b71c2b67a/config/loader_hcl.go#L1215-L1237
func unwrapHCLObjectKeysFromJSON(item *hclast.ObjectItem, depth int) {
	if len(item.Keys) > depth && item.Keys[0].Token.JSON {
		for len(item.Keys) > depth {
			// Pop off the last key
			n := len(item.Keys)
			key := item.Keys[n-1]
			item.Keys[n-1] = nil
			item.Keys = item.Keys[:n-1]

			// Wrap our value in a list
			item.Val = &hclast.ObjectType{
				List: &hclast.ObjectList{
					Items: []*hclast.ObjectItem{
						{
							Keys: []*hclast.ObjectKey{key},
							Val:  item.Val,
						},
					},
				},
			}
		}
	}
}
