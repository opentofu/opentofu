// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cliconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/hashicorp/hcl"
	hclast "github.com/hashicorp/hcl/hcl/ast"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// decodeOCIRegistrySettingsFromConfig finds any oci_registries blocks in the
// given configuration and returns a single OCIRegistries object representing
// the merger of all of them.
//
// Returns nil if there are not any oci_registries blocks in the file at all.
// In that case the caller might decide to try searching for a Docker CLI
// configuration file to use instead, using
// [impliedOCIRegistrySettingsFromDockerConfig].
//
// Multiple blocks are primarily allowed to permit splitting the settings over
// multiple files, such as if someone would rather write a separate file for
// each OCI registry they work with, but we allow multiple blocks in the same
// file (with the same merging behavior) for consistency's sake.
func decodeOCIRegistrySettingsFromConfig(hclFile *hclast.File) (*OCIRegistries, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// We'll merge each new oci_registry block into this as we go, since
	// that allows us to reuse the merge method both in here and in Config.Merge.
	//
	// Blocks that we encounter later override ones we'd previously visited.
	var ret *OCIRegistries

	root, ok := hclFile.Node.(*hclast.ObjectList)
	if !ok {
		// If we get here then the file is too invalid to even have a
		// root node, but the caller should've already reported that
		// on the initial parse so we won't duplicate the error.
		return ret, nil
	}

	// The following is performing an "HCL 2"-like decode using the HCL 1
	// API, continuing the existing precedent of not introducing anything
	// new that might be hard to migrate to HCL 2 later.
	for _, block := range root.Items {
		if block.Keys[0].Token.Value() != "oci_registries" {
			continue
		}

		isJSON := block.Keys[0].Token.JSON
		if block.Assign.Line != 0 && !isJSON {
			// Seems to be an attribute rather than a block
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid oci_registries block",
				fmt.Sprintf("The oci_registries block at %s must not be introduced with an equals sign.", block.Pos()),
			))
			continue
		}
		if len(block.Keys) > 1 && !isJSON {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid oci_registry block",
				fmt.Sprintf("The oci_registries block at %s must not have any labels.", block.Pos()),
			))
		}

		cfg := &OCIRegistries{}

		// The structure _inside_ an oci_registry block is straightforward
		// enough for us to decode it using the HCL 1 decoder without permitting
		// structures that would be super hard to migrate to HCL 2 later.
		type blockInnards struct {
			CredentialHelper        string            `hcl:"credential_helper"`
			DomainCredentialHelpers map[string]string `hcl:"domain_credential_helpers"`
			// The "StaticCredentials" field is intentionally not supported here
			// because we don't want to encourage folks to duplicate their
			// static credentials from their Docker CLI configuration. That field
			// is there only to allow us to automatically discover credentials that
			// were previously written by "docker login", for folks who have both
			// OpenTofu and Docker CLI installed.
		}
		var innards blockInnards
		if err := hcl.DecodeObject(&innards, block.Val); err != nil {
			// HCL 1 doesn't tend to generate very high-quality error messages, but
			// we'll accept it as good enough with some extra context added on here.
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid oci_registries block",
				fmt.Sprintf("The oci_registries block at %s has invalid content: %s.", block.Pos(), err),
			))
		}

		ret = ret.merge(cfg)
	}

	return ret, diags
}

// impliedOCIRegistrySettingsFromDockerLikeConfig makes a best effort to construct
// an [OCIRegistries] based on the configuration files and environment variables
// normally used and updated by other tools in the container ecosystem, including
// Podman and Docker CLI.
//
// This directly accesses the relevant environment variables and files on disk
// to encapsulate all of the Podman/Docker-specific details. Use
// [impliedOCIRegistrySettingsFromDockerConfigFile] instead if you've already
// got the source code of a Docker CLI-like config file loaded into a byte slice.
//
// Returns nil if there does not appear to be any ambient configuration available.
//
// The goal here is to give a better "out of box" experience for anyone who
// already has Docker CLI or similar installed and configured to interact with the
// OCI registries they want to use, and in particular so that we can make
// use of credentials previously issued by "docker login"/"podman login" to avoid our
// users needing to duplicate those secrets in another place.
//
// This is not a fully-comprehensive implementation of the Docker CLI config language,
// and since the evolution of the Docker CLI language is not under OpenTofu's
// control this function prefers to ignore anything unexpected rather than
// returning an error. It does, however, produce some log output for certain
// problems to make it easier to handle bug reports that suggest our
// interpretation of this language is not sufficiently compatible with the
// official Docker/Moby implementation.
func impliedOCIRegistrySettingsFromDockerLikeConfig() *OCIRegistries {
	// We try two different search patterns for Docker CLI-like config
	// files. First we try a generic form pioneered by Podman that has
	// been adopted by some other software in the ecosystem, and then
	// we fall back to the Docker-CLI-specific search pattern if the
	// generic form is unsuccessful.
	filename, src, err := tryFindContainerCLIConfigFile()
	if err != nil {
		if !os.IsNotExist(err) {
			// We only warn if it's some error other than "No such file or directory"
			// because it's very reasonable for someone to be using OpenTofu without
			// ever using another tool to log in first, and so we don't make noise in
			// that case.
			log.Printf("[WARN] Failed to read Docker-CLI-style container registry configuration from %s: %s", filename, err)
		}

		filename, src, err = tryFindDockerCLIConfigFile()
		if err != nil {
			if !os.IsNotExist(err) {
				// Again, we only warn if we found a file but failed to read it.
				log.Printf("[WARN] Failed to read Docker CLI configuration from %s: %s", filename, err)
			}
			// If we didn't find either file then there's nothing to load.
			// No logging for this case because we don't have a clear signal
			// of user intent to use either of these files, and irrelevant
			// log warnings tend to cause distraction when debugging.
			return nil
		}
	}

	// If we get here then we're definitely attempting to populate our OCI
	// registry settings from the selected config file, so we'll announce
	// that to help folks with debugging downstream behavior.
	log.Printf("[INFO] Inferring OCI registry configuration from Docker CLI-style config at %s", filename)

	return impliedOCIRegistrySettingsFromDockerLikeConfigSource(src)
}

// tryFindContainerCLIConfigFile tries to find a Docker CLI-style config file
// in the non-Docker-CLI-specific location that was pioneered by Podman and
// then followed by some other tools in the container ecosystem.
//
// Specifically this looks for ${XDG_RUNTIME_DIR}/containers/auth.json on
// Linux and $HOME/.config/containers/auth.json on other platforms, along with
// some other quirkly special cases, following Podman's approach to maximize
// compatibility.
//
// If no such file is available at all, returns an error that would cause
// [os.IsNotExist] to return true. Can potentially return other errors if the
// file does exist but is somehow unreadable. If the error is nil or does not pass
// [os.IsNotExist] then the first return value is guaranteed to contain the
// filename we tried to read, for use in logs.
func tryFindContainerCLIConfigFile() (string, []byte, error) {
	// We gather up all of the other "ambient" information here just so that
	// we can unit test containerCLIConfigFilePaths under the assumption
	// that it will behave like a pure function.
	goos := runtime.GOOS
	xdgConfigHome := os.Getenv("XDG_CONFIG_HOME")
	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	containersRootlessUID := os.Getenv("_CONTAINERS_ROOTLESS_UID")
	uid := os.Getuid()
	euid := os.Getegid()

	// The following mimics Podman's implementation of finding the home
	// directory, which uses some different details than OpenTofu otherwise
	// does, and is also different to how Docker CLI does it (which we
	// mimic separately in tryFindDockerCLIConfigFile.)
	home := os.Getenv("HOME")
	if home == "" {
		u, err := user.LookupId(strconv.Itoa(containerCLIUserIDForConfigFileLookup(goos, containersRootlessUID, uid, euid)))
		if err != nil {
			log.Printf("[WARN] Can't find user home directory using a Podman-like strategy: %s", err)
			return "", nil, os.ErrNotExist // We can't find anything at all if we don't even have a home directory
		}
		home = u.HomeDir
	}
	filenames := containerCLIConfigFilePaths(
		runtime.GOOS,
		home,
		xdgConfigHome,
		xdgRuntimeDir,
		uid,
	)
	for _, filename := range filenames {
		src, err := os.ReadFile(filename)
		if os.IsNotExist(err) {
			// If the file doesn't exist at all then we'll try the next one. Otherwise we'll fail immediately.
			continue
		}
		return filename, src, err
	}
	// If we fall out here then we have no file to use at all.
	return "", nil, os.ErrNotExist
}

// containerCLIConfigFilePaths decides which paths [tryFindContainerCLIConfigFile]
// shuld attempt to read from based on the current Go runtime operating system
// and on two of the environment variables defined in the XDG Base Directory
// specification.
//
// If the result is an zero-leng slice then no path is available on the current
// system.
//
// This aims to match the logic used by Podman for the same purpose, and is
// factored out only to allow testing it with different inputs regardless
// of the environment where the tests are running.
//
// This logic is factored out into a pure function to aid in testing, but beware
// that the result returns paths using the host platform's filepath syntax
// so tests will still need to normalize those in order to be portable.
func containerCLIConfigFilePaths(goos, home, xdgConfigHome, xdgRuntimeDir string, uid int) []string {
	// These are all defined to match the config path builder in
	// github.com/containers/image, which is the library that Podman uses
	// to find its file.
	const (
		configFilename       = "auth.json"
		configSubdir         = "containers"
		xdgConfigHomeDefault = ".config"
	)

	var ret []string
	switch goos {
	case "linux":
		// On Linux Podman uses XDG_RUNTIME_DIR as a base directory if it's set, or a hard-coded path
		// containing the user ID if it's not.
		if xdgRuntimeDir != "" {
			ret = append(ret, filepath.Join(xdgRuntimeDir, configSubdir, configFilename))
		} else {
			ret = append(ret, filepath.Join("/run/containers", strconv.Itoa(uid), configFilename))
		}
	default:
		// Podman uses a hard-coded path relative to the home directory on everything except Linux.
		ret = append(ret, filepath.Join(home, xdgConfigHomeDefault, configSubdir, configFilename))
	}

	// On all platforms Podman additionally searches relative to XDG_CONFIG_HOME.
	// Note that it doesn't actually fully implement the XDG spec, because it doesn't
	// also search the XDG_CONFIG_DIR directories. We match that quirk for compatibilty.
	// On non-Linux platforms it uses XDG only in this one case, and note that this
	// can potentially generate a duplicate of the path we already generated on non-Linux
	// platforms above, which is weird but also true of Podman.
	if xdgConfigHome == "" {
		// (Intentionally using a hard-coded structure rather than the go-homedirs library
		// OpenTofu uses elsewhere because we want to match Podman closely.)
		xdgConfigHome = filepath.Join(home, xdgConfigHomeDefault)
	}
	ret = append(ret, filepath.Join(xdgConfigHome, configSubdir, configFilename))

	// Podman also adds the Docker CLI config to its search paths but we handle that
	// separately in tryFindDockerCLIConfigFile so that we can follow Docker CLI's
	// slightly-different conventions more closely.

	return ret
}

func containerCLIUserIDForConfigFileLookup(goos, containersRootlessUID string, uid, euid int) int {
	// This mimics some trickery that Podman does, to match its behavior
	// as closely as possible.
	if goos == "linux" {
		if containersRootlessUID != "" {
			if u, err := strconv.Atoi(containersRootlessUID); err == nil {
				return u
			}
		}
		return euid
	}
	// All other platforms just use the plain uid.
	return uid
}

// tryFindDockerCLIConfigFile is like [tryFindContainerCLIConfigFile] but
// instead searches for the config file at the Docker-specific path where
// Docker CLI would look for it: ${DOCKER_CONFIG}/config.json or
// ${HOME}/.docker/config.json if DOCKER_CONFIG isn't set.
//
// Return values follow the same contract as for [tryFindContainerCLIConfigFile].
func tryFindDockerCLIConfigFile() (string, []byte, error) {
	// These are all defined to match the config loader in docker/cli.
	const (
		dockerConfigDirOverrideEnv = "DOCKER_CONFIG"
		dockerConfigFilename       = "config.json"
		defaultDockerConfigDir     = ".docker"
	)

	configDir := os.Getenv(dockerConfigDirOverrideEnv)
	if configDir == "" {
		// The following matches how Docker CLI determines the home directory,
		// which is slightly different to how OpenTofu typically does it.
		home, _ := os.UserHomeDir()
		if home == "" && runtime.GOOS != "windows" {
			if u, err := user.Current(); err == nil {
				home = u.HomeDir
			}
		}
		if home == "" {
			// If we can't find the home directory then we have no config to load.
			// No logging for this case because we don't yet have sufficient evidence
			// of the user's intent for OpenTofu to find a Docker CLI config.
			return "", nil, os.ErrNotExist
		}
		configDir = filepath.Join(home, defaultDockerConfigDir)
	}

	filename := filepath.Join(configDir, dockerConfigFilename)
	src, err := os.ReadFile(filename)
	return filename, src, err
}

// impliedOCIRegistrySettingsFromDockerConfigFile is the main decoding logic for
// impliedOCIRegistrySettingsFromDockerConfig, separated to make it easier to
// test without having to fake out the environment and home directory.
func impliedOCIRegistrySettingsFromDockerLikeConfigSource(src []byte) *OCIRegistries {
	// Docker CLI's config file format is JSON-based, documented here:
	//    https://docs.docker.com/reference/cli/docker/#docker-cli-configuration-file-configjson-properties
	// We are only interested in a small subset of the format that Docker CLI
	// would use for registry authentication:
	//   - "auths" for static credentials, populated by "docker login"
	//   - "credHelpers" as a map from registry hostname to credentials helper plugin names
	//   - "credsStore" as a default credentials helper for any host not mentioned in "credHelpers"

	type jsonAuthConfig struct {
		Auth          []byte `json:"auth"`          // encoding/json automatically base64-decodes for []byte
		RegistryToken string `json:"registrytoken"` // JSON property name is all lowercase in Docker CLI, too
	}
	type jsonConfig struct {
		Auths       map[string]*jsonAuthConfig `json:"auths"`
		CredStore   string                     `json:"credsStore,omitempty"`
		CredHelpers map[string]string          `json:"credHelpers,omitempty"`
	}
	var config jsonConfig
	err := json.Unmarshal(src, &config)
	if err != nil {
		// Just a debugging aid for if someone finds that this can't handle
		// something that Docker CLI would normally be able to handle.
		log.Printf("[WARN] Failed to decode Docker CLI config file: %s", err)
		return nil
	}

	// At this point we are definitely going to return an object of some sort,
	// to represent that we found and tried to use a Docker CLI file even
	// if there was nothing in it that is relevant to us.
	ret := &OCIRegistries{
		CredentialHelper: config.CredStore,
	}
	if credHelpersCount := len(config.CredHelpers); credHelpersCount != 0 {
		ret.DomainCredentialHelpers = make(map[string]string, credHelpersCount)
		for k, v := range config.CredHelpers {
			ret.DomainCredentialHelpers[k] = v
		}
	}
	if staticCredsCount := len(config.Auths); staticCredsCount != 0 {
		ret.StaticCredentials = make(map[string]OCIRegistryAuth, staticCredsCount)
		for domain, auth := range config.Auths {
			switch {
			case auth.RegistryToken != "":
				ret.StaticCredentials[domain] = OCIRegistryAuth{
					RegistryToken: auth.RegistryToken,
				}
			case len(auth.Auth) != 0:
				// These ones are a little more interesting: the file on disk
				// has a base64-encoded username:password string, but clients
				// want the decoded credentials. Since we decoded into []byte
				// the JSON package has already done the base64 decoding, but
				// we still need to split the resulting string.
				username, password, valid := bytes.Cut(auth.Auth, []byte{':'})
				if !valid {
					log.Printf("[WARN] Docker CLI configuration has invalid auth string for %s; ignoring", domain)
					continue
				}
				ret.StaticCredentials[domain] = OCIRegistryAuth{
					Username: string(username),
					Password: string(password),
				}
			default:
				log.Printf("[WARN] Docker CLI configuration has unsupported authentication method for %s; ignoring", domain)
			}
		}
	}
	return ret
}

// OCIRegistries corresponds to the "oci_registry" block type in the CLI
// config, which allows configuring global settings needed for interacting
// with OCI registries regardless of use-case.
//
// Other specific OpenTofu features make use of this information. For example,
// if the provider_installation block calls for installing some or all providers
// from an "OCI mirror" source then these settings will be used to decide how
// to authenticate to the underlying OCI registries.
//
// A subset of these settings can also potentially be inferred from the
// conventional Docker CLI configuration at ~/.docker/config.json or
// similar, as a shortcut for anyone who also has Docker CLI installed and
// has already configured it. That shortcut is allowed only when the
// TF_CLI_CONFIG_FILE environment variable isn't set, because that environment
// variable forces _only_ using the specified configuration file.
type OCIRegistries struct {
	// CredentialHelper is the name of a credentials helper program to use
	// to obtain credentials for any repository whose domain isn't included
	// in either DomainCredentialHelpers or DomainAuths.
	//
	// This must be a program supporting the protocol defined in:
	//     https://github.com/docker/docker-credential-helpers
	//
	// The empty string represents that no credentials helper is available.
	CredentialHelper string

	// CredentialHelpers is a map from registry domain to a credential
	// helper program to use for that domain.
	//
	// The values in this map have the same syntax and meaning as for
	// the CredentialHelper field, overriding the credential helper for
	// just one domain.
	DomainCredentialHelpers map[string]string

	// StaticCredentials is a map from registry domain to some static
	// authentication credentials to use for that hostname. This is
	// used only if there isn't an element with the same key in
	// DomainCredentialHelpers, because credential helpers take priority
	// over static credentials according to the Docker CLI configuration
	// documentation:
	//     https://docs.docker.com/reference/cli/docker/#credential-store-options
	//
	// This is populated only when the object was constructed from the
	// Docker CLI configuration file. The OpenTofu CLI configuration format
	// does not allow inline static credentials in order to avoid
	// secret sprawl: users must either use credential helpers or
	// must place their static credentials in the Docker CLI configuration
	// e.g. by using "docker login".
	StaticCredentials map[string]OCIRegistryAuth
}

// merge returns an [OCIRegistries] object representing the merger of the
// two that are given.
//
// If either is nil then the other is returned directly, without allocating
// a new object. If both are nil then the result is also nil.
func (or *OCIRegistries) merge(other *OCIRegistries) *OCIRegistries {
	// If we only have zero or one existing objects then we'll save
	// an allocation by just returning what we were given.
	// (This is primarily for decodeOCIRegistrySettingsFromConfig's
	// benefit, so it can allocate exactly one object in the common
	// case a given CLI configuration file has only one block.)
	switch {
	case or == nil:
		return other // NOTE: intentionally returns nil if both are nil
	case other == nil:
		return or
	}

	// Otherwise we're constructing a new object to avoid mutating
	// either of the given ones.
	ret := &OCIRegistries{}
	if other.CredentialHelper != "" {
		ret.CredentialHelper = other.CredentialHelper
	} else {
		ret.CredentialHelper = or.CredentialHelper
	}

	ret.DomainCredentialHelpers = mergeMaps(or.DomainCredentialHelpers, other.DomainCredentialHelpers)
	ret.StaticCredentials = mergeMaps(or.StaticCredentials, other.StaticCredentials)

	return ret
}

// OCIRegistryAuth represents static authentication credentials loaded from
// the Docker configuration file.
type OCIRegistryAuth struct {
	// Username and Password are static basic authentication credentials to
	// use for the registry.
	//
	// These are ignored when RegistryToken is also set.
	Username, Password string

	// RegistryToken is a static bearer token to use for the registry.
	//
	// This takes priority over Username/Password if all are set in
	// the source configuration.
	RegistryToken string
}

func mergeMaps[K comparable, T any](a, b map[K]T) map[K]T {
	maxPossibleLength := len(a) + len(b)
	if maxPossibleLength == 0 {
		return nil
	}

	// maxPossibleLength will be an over-estimate if there are
	// any overridden elements in b, but that's okay because
	// this is only a hint anyway.
	ret := make(map[K]T, maxPossibleLength)
	for k, v := range a {
		ret[k] = v
	}
	for k, v := range b {
		ret[k] = v
	}
	return ret
}
