// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package getproviders

import (
	"context"
	"fmt"
	"time"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/depsrccfgs"
)

type mappingConfigSource struct {
	mainSource Source
	configs    []*depsrccfgs.Config
	env        MappingConfigSourceEnv
}

// NewMappingConfigSource returns a source that wraps another source while
// preferring to use provider address mappings from the given mapping
// configuration files if any suitable rules are present.
func NewMappingConfigSource(mainSource Source, configs []*depsrccfgs.Config, env MappingConfigSourceEnv) Source {
	return &mappingConfigSource{mainSource, configs, env}
}

// AvailableVersions implements Source.
func (m *mappingConfigSource) AvailableVersions(ctx context.Context, provider addrs.Provider) (VersionList, Warnings, error) {
	realSource, err := m.selectRealSource(provider)
	if err != nil {
		return nil, nil, err
	}
	return realSource.AvailableVersions(ctx, provider)
}

// ForDisplay implements Source.
func (m *mappingConfigSource) ForDisplay(provider addrs.Provider) string {
	realSource, err := m.selectRealSource(provider)
	if err != nil {
		return fmt.Sprintf("undecided source for %s", provider.String())
	}
	return realSource.ForDisplay(provider)
}

// PackageMeta implements Source.
func (m *mappingConfigSource) PackageMeta(ctx context.Context, provider addrs.Provider, version Version, target Platform) (PackageMeta, error) {
	realSource, err := m.selectRealSource(provider)
	if err != nil {
		return PackageMeta{}, err
	}
	return realSource.PackageMeta(ctx, provider, version, target)
}

func (m *mappingConfigSource) selectRealSource(provider addrs.Provider) (Source, error) {
	// We try each configuration in turn, prioritizing the best result
	// from the first configuration that generates any result at all.
	//
	// This means that rule specificity only applies within each individual
	// file: a less specific rule in an earlier file can still "win" over
	// a more specific rule in a later file, to allow override files to
	// make broad unilateral remaps of what might be specified in a more
	// fine-grain way in a lower-priority file.
	for _, config := range m.configs {
		var bestRule *depsrccfgs.ProviderPackageRule

		for _, rule := range config.ProviderPackageRules {
			if !rule.MatchPattern.Matches(provider) {
				continue
			}
			if bestRule != nil && bestRule.MatchPattern.Specificity() > rule.MatchPattern.Specificity() {
				// (the depssrccfgs package must guarantee that there are
				// never two rules of the same specificity matching the same
				// provider, treating such a configuration as invalid.)
				continue // more specific rules always "win"
			}
			bestRule = rule
		}

		if bestRule != nil {
			return m.sourceForRule(bestRule)
		}
	}

	// If all else fails we fall back to the "main" source, which will
	// presumably deal with all of the normal provider installation rules.
	return m.mainSource, nil
}

func (m *mappingConfigSource) sourceForRule(rule *depsrccfgs.ProviderPackageRule) (Source, error) {
	// For now this just awkwardly borrows our sources that were originally
	// designed to make sense for the CLI configuration's provider installation
	// settings, which is awkward because they were designed to be instantated
	// just once on startup rather than dynamically "just in time" for
	// installation. But this is just a prototype, so it'll do for now.

	switch mapper := rule.Mapper.(type) {
	case *depsrccfgs.ProviderPackageDirectMapper:
		// For now "direct" really means "use the main source", so that a
		// more specific rule can use it as an exclusion from a less specific
		// rule where e.g. one specific provider ought to still be installed
		// in whatever is the "normal" way.
		//
		// "direct" is probably not actually the right name for this since in
		// the CLI configuration layer that _always_ means "use the registry
		// protocol" but here it means to yield to whatever the CLI
		// configuration said to do. It'll do for now, though.
		return m.mainSource, nil
	case *depsrccfgs.ProviderPackageNetworkMirrorMapper:
		// TODO: Route our central HTTP client and auth credentials object into
		// here so this can agree with the behavior of the rest of the system.
		return NewHTTPMirrorSource(mapper.BaseURL, nil, 10*time.Second), nil
	case *depsrccfgs.ProviderPackageOCIMapper:
		return NewOCIRegistryMirrorSource(
			mapper.RepositoryAddrFunc,
			m.env.OCIRepositoryStore,
		), nil
	default:
		// The cases above should be exhaustive for all implementations of
		// [depsrccfgs.ProviderPackageMapper].
		return nil, fmt.Errorf("don't know how to handle %T", mapper)
	}
}

type MappingConfigSourceEnv interface {
	OCIRepositoryStore(ctx context.Context, registryDomain, repositoryName string) (OCIRepositoryStore, error)
}
