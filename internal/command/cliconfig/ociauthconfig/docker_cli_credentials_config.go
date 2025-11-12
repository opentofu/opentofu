// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ociauthconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"strings"
)

// FindDockerCLIStyleCredentialsConfigs attempts to automatically discover Docker CLI-style
// credentials configurations in the same locations searched by various other tools in
// the OCI ecosystem, as documented in
// https://github.com/containers/image/blob/main/docs/containers-auth.json.5.md .
func FindDockerCLIStyleCredentialsConfigs(ctx context.Context, env ConfigDiscoveryEnvironment) ([]CredentialsConfig, error) {
	var ret []CredentialsConfig
	var err error
	prevPath := ""
	for _, filePath := range dockerCLIStyleAuthFileSearchLocations(env) {
		if filePath == prevPath {
			// The rules for generating search paths sometimes cause two sequential
			// equal paths on some platforms, which causes us to end up in here.
			// We'll skip the duplicate to avoid wasting time reading and querying
			// the same file twice.
			continue
		}
		prevPath = filePath

		src, readErr := env.ReadFile(ctx, filePath)
		if readErr != nil {
			if !os.IsNotExist(readErr) {
				err = errors.Join(err, fmt.Errorf("reading %s: %w", filePath, readErr))
			}
			continue
		}
		cfg, parseErr := newDockerCLIStyleCredentialsConfig(src, filePath)
		if parseErr != nil {
			err = errors.Join(err, fmt.Errorf("parsing %s: %w", filePath, readErr))
			continue
		}
		ret = append(ret, cfg)
	}
	return ret, err
}

// FixedDockerCLIStyleCredentialsConfigs tries to use the given fixed set of filepaths
// all as Docker CLI-style credentials configuration files.
//
// This is an alternative to [FindDockerCLIStyleCredentialsConfigs] for when someone
// has explicitly configured the files to look in, overriding the automatic discovery
// process.
//
// We assume these files have been specified directly by the operator rather than by
// automatic discovery, and so any file being absent is treated as an error in this
// case, whereas automatic discovery just ignores file paths referring to nonexistent
// files.
func FixedDockerCLIStyleCredentialsConfigs(ctx context.Context, filePaths []string, env ConfigDiscoveryEnvironment) ([]CredentialsConfig, error) {
	var ret []CredentialsConfig
	var err error
	for _, filePath := range filePaths {
		src, readErr := env.ReadFile(ctx, filePath)
		if readErr != nil {
			err = errors.Join(err, fmt.Errorf("reading %s: %w", filePath, readErr))
			continue
		}
		cfg, parseErr := newDockerCLIStyleCredentialsConfig(src, filePath)
		if parseErr != nil {
			err = errors.Join(err, fmt.Errorf("parsing %s: %w", filePath, parseErr))
			continue
		}
		ret = append(ret, cfg)
	}
	return ret, err
}

type dockerCLIStyleCredentialsConfig struct {
	filename string
	content  dockerCLIStyleConfigFile
}

func newDockerCLIStyleCredentialsConfig(src []byte, filename string) (CredentialsConfig, error) {
	// At this stage we just perform the JSON unmarshal step to confirm
	// that the given file seems to be something sensible. We'll wait
	// until we're actually asked a question before analyzing further,
	// because most OpenTofu commands don't interact with OCI auth at
	// all so we'd be wasting our time if it isn't going to be used anyway.
	ret := &dockerCLIStyleCredentialsConfig{
		filename: filename,
	}
	err := json.Unmarshal(src, &ret.content)
	if err != nil {
		return nil, fmt.Errorf("invalid JSON syntax: %w", err)
	}
	return ret, nil
}

func (c *dockerCLIStyleCredentialsConfig) CredentialsSourcesForRepository(_ context.Context, registryDomain string, repositoryPath string) iter.Seq2[CredentialsSource, error] {
	return func(yield func(CredentialsSource, error) bool) {
		// Our main work is to test all of the properties in "auths" to see if their
		// names match the requested domain/repository, and return any that do
		// as static credentials.
		for propName, auth := range c.content.Auths {
			if auth == nil || len(auth.Auth) == 0 {
				// We silently ignore null or invalid auth objects, since
				// some tools apparently generate such entries when using
				// their "login" commands when a credential helper is
				// configured, and thus when the actual username/password
				// were stored in the credential helper instead of directly
				// in the configuration file.
				continue
			}
			spec := ContainersAuthPropertyNameMatch(propName, registryDomain, repositoryPath)
			if spec == NoCredentialsSpecificity {
				continue // doesn't match at all
			}
			var source CredentialsSource
			var err error
			// The auth.Auth value was given in the source JSON as a base64-encoded
			// username:password string, but since we decoded it into []byte the
			// encoding/json library already performed base64 decoding and so we
			// only need to split it.
			username, password, hasColon := strings.Cut(string(auth.Auth), ":")
			if hasColon {
				source = NewStaticCredentialsSource(
					Credentials{
						username: username,
						password: password,
					},
					spec,
				)
			} else {
				err = fmt.Errorf("auth object for %q in %s does not have base64-encoded username:password pair", propName, c.filename)
			}
			keepGoing := yield(source, err)
			if !keepGoing {
				return
			}
		}
		// If there is a domain-specific credential helper for the requested domain
		// in the "credHelpers" object then we'll return that too.
		if helperName := c.content.CredHelpers[registryDomain]; helperName != "" {
			keepGoing := yield(NewDockerCredentialHelperCredentialsSource(helperName, "https://"+registryDomain, DomainCredentialsSpecificity), nil)
			if !keepGoing {
				return
			}
		}
		// If there is a global credential helper ("credsStore" in the config format)
		// then we'll return it regardless of which repository we're being asked about.
		if helperName := c.content.CredsStore; helperName != "" {
			//nolint:staticcheck // The following keepGoing test is redundant as currently written, but included as a reminder that this is needed if we add another case after this one in future
			keepGoing := yield(NewDockerCredentialHelperCredentialsSource(helperName, "https://"+registryDomain, GlobalCredentialsSpecificity), nil)
			if !keepGoing {
				return
			}
		}
	}
}

func (c *dockerCLIStyleCredentialsConfig) CredentialsConfigLocationForUI() string {
	// We just use the filename directly for these, since that's the most
	// specific reference we have.
	return c.filename
}

type dockerCLIStyleConfigFile struct {
	Auths       map[string]*dockerCLIStyleAuth `json:"auths"`       // domain-specific or repository-specific static credentials
	CredHelpers map[string]string              `json:"credHelpers"` // domain-specific credential helpers
	CredsStore  string                         `json:"credsStore"`  // global credential helper
}

type dockerCLIStyleAuth struct {
	Auth []byte `json:"auth"`
}

// ContainersAuthPropertyNameMatch determines to what extent a property name from the "auths"
// object in a Docker/Podman/Buildah/etc-style auth config file matches the given registry domain
// and repository path, if at all.
//
// "Containers auth" in this name is a reference to the following document:
// https://github.com/containers/image/blob/c30cc7a54783122c0168d8ad77f712c2469c496c/docs/containers-auth.json.5.md
//
// Returns [NoCredentialsSpecificity] if it doesn't match at all, [DomainCredentialsSpecificity]
// if only the registry domain matches, or a greater [CredentialsSpecificity] value if both
// the domain and at least one path segment matches.
//
// This "containers auth" address format is actually an extension of the original Docker CLI
// format. Docker CLI only supports whole-registry-domain auths, while this later specification
// extended that format to support repository-specific configurations that are matched by repository
// path prefix, preferring the most specific match. Docker-compatible configuration files can
// therefore produce only either [NoCredentialsSpecificity] or [DomainCredentialsSpecificity] results.
//
// This is exported because OpenTofu's package cliconfig uses the same syntax and matching rules
// for its own "oci_credentials" blocks, implemented using this same function for consistency.
func ContainersAuthPropertyNameMatch(authsPropertyName string, wantRegistryDomain, wantRepositoryPath string) CredentialsSpecificity {
	if authsPropertyName == "" {
		return NoCredentialsSpecificity // Invalid
	}
	var gotDomain, gotRepositoryPath string
	if strings.Count(authsPropertyName, "/") > 0 {
		var err error
		gotDomain, gotRepositoryPath, err = ParseRepositoryAddressPrefix(authsPropertyName)
		if err != nil {
			// Since these Docker CLI-style config files are not exclusively for OpenTofu,
			// we just silently ignore a property whose name we can't make sense of
			// in case a future version of this format adds something new that we
			// don't yet support.
			return NoCredentialsSpecificity // Invalid
		}
	} else {
		gotDomain = authsPropertyName
	}
	if gotDomain != wantRegistryDomain {
		return NoCredentialsSpecificity // does not match
	}
	if gotRepositoryPath == "" {
		return DomainCredentialsSpecificity // matches only the domain
	}
	// If authsPropertyName includes a path (that is: if gotRepositoryPath != "")
	// then we need to test if gotRepositoryPath has at most as many segments
	// as wantRepositoryPath, and that those segments all match.
	wantSegCount := strings.Count(wantRepositoryPath, "/") + 1
	gotSegCount := strings.Count(gotRepositoryPath, "/") + 1
	if gotSegCount > wantSegCount {
		// The path in authsPropertyName has too many segments to possibly
		// match wantRepositoryPath.
		return NoCredentialsSpecificity
	}
	wantRemain := wantRepositoryPath
	gotRemain := gotRepositoryPath
	for range wantSegCount {
		var thisWant, thisGot string
		var moreGot bool
		thisWant, wantRemain, _ = strings.Cut(wantRemain, "/")
		thisGot, gotRemain, moreGot = strings.Cut(gotRemain, "/")
		if thisWant != thisGot {
			// If we find a mismatch before we run out of "got" then
			// only a prefix of authsPropertyName matches the wanted
			// address, which means that this auth configuration is
			// ineligible for the given repository.
			return NoCredentialsSpecificity
		}
		if !moreGot {
			break // no more segments to compare, so we've found at least a valid prefix match
		}
	}
	// If we get here without returning early then the whole of gotRepositoryPath
	// matched, so our specificity is the number of segments in that path.
	return RepositoryCredentialsSpecificity(uint(gotSegCount))
}

func dockerCLIStyleAuthFileSearchLocations(env ConfigDiscoveryEnvironment) []string {
	// The following enumerates all of the search locations described in the following
	// document, taking into account the environment's reported operating system name:
	// https://github.com/containers/image/blob/main/docs/containers-auth.json.5.md
	os := env.OperatingSystemName()
	homeDir := env.UserHomeDirPath()
	isMacOS := os == "darwin"
	isWindows := os == "windows"
	isLinux := os == "linux"
	var ret []string

	if isLinux {
		if xdgRunDir := env.EnvironmentVariableVal("XDG_RUNTIME_DIR"); xdgRunDir != "" {
			ret = append(ret, filepath.Join(xdgRunDir, "containers", "auth.json"))
		}
	} else if isWindows || isMacOS {
		ret = append(ret, filepath.Join(homeDir, ".config", "containers", "auth.json"))
	}

	xdgConfigHome := env.EnvironmentVariableVal("XDG_CONFIG_HOME")
	if xdgConfigHome == "" {
		xdgConfigHome = filepath.Join(homeDir, ".config") // as required by XDG Base Directory spec
	}
	ret = append(ret, filepath.Join(xdgConfigHome, "containers", "auth.json")) // this might duplicate the primary location from above
	ret = append(ret, filepath.Join(homeDir, ".docker", "config.json"))
	ret = append(ret, filepath.Join(homeDir, ".dockercfg"))
	return ret
}
