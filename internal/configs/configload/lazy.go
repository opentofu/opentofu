// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configload

import (
	"context"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
)

// lazyLoader implements Loader. When created, it does not initialise the
// actual implementation of the loader but instead defers that to the first
// call to its methods.
//
// Each Loader method that returns no error will return a sane default that the system
// is known to work with in case the underlying loader initialisation will fail during
// those methods' calls.
// If the initialisation fails during the call on these methods, the underlying loader will
// remain nil, retrying to do so on subsequent calls until a method that returns diagnostics
// is finally called, returning the root cause error.
type lazyLoader struct {
	cached Loader

	cfg *Config
}

func NewLazy(c *Config) Loader {
	return &lazyLoader{
		cfg: c,
	}
}

// Initialise provides a way for the users of Loader to forcefully initialise the instance
// and not wait for the its methods to be called to do it.
// This is useful in cases where the lazy loading of the loader (by calling member methods), could
// create unexpected issues and unwanted errors in unwanted places.
func Initialise(l Loader) (Loader, error) {
	ll, ok := l.(*lazyLoader)
	if !ok {
		return l, nil
	}
	return ll.init()
}

// init initialises the underlying Loader by calling NewLoader.
func (c *lazyLoader) init() (Loader, error) {
	if c.cached == nil {
		newLoader, err := NewLoader(c.cfg)
		if err != nil {
			return nil, err
		}
		c.cached = newLoader
	}
	return c.cached, nil
}

// ImportSources implements Loader
func (c *lazyLoader) ImportSources(sources map[string][]byte) {
	l, err := c.init()
	if err != nil {
		return
	}
	l.ImportSources(sources)
}

// ImportSourcesFromSnapshot implements Loader
func (c *lazyLoader) ImportSourcesFromSnapshot(snap *Snapshot) {
	l, err := c.init()
	if err != nil {
		return
	}
	l.ImportSourcesFromSnapshot(snap)
}

// IsConfigDir implements Loader
func (c *lazyLoader) IsConfigDir(path string) bool {
	l, err := c.init()
	if err != nil {
		return true // The same default with the previous version in Meta structure
	}
	return l.IsConfigDir(path)
}

// ModulesDir implements Loader
func (c *lazyLoader) ModulesDir() string {
	l, err := c.init()
	if err != nil {
		return ""
	}
	return l.ModulesDir()
}

// RefreshModules implements Loader
func (c *lazyLoader) RefreshModules() error {
	l, err := c.init()
	if err != nil {
		return err
	}
	return l.RefreshModules()
}

// Sources implements Loader
func (c *lazyLoader) Sources() map[string]*hcl.File {
	l, err := c.init()
	if err != nil {
		return nil
	}
	return l.Sources()
}

// LoadConfig implements Loader
func (c *lazyLoader) LoadConfig(ctx context.Context, rootDir string, call configs.StaticModuleCall) (*configs.Config, hcl.Diagnostics) {
	l, err := c.init()
	if err != nil {
		return nil, hcl.Diagnostics{
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Config loader init failed",
				Detail:   fmt.Sprintf("Failed to initialise a config loader: %s", err),
			},
		}
	}
	return l.LoadConfig(ctx, rootDir, call)
}

// LoadConfigWithTests implements Loader
func (c *lazyLoader) LoadConfigWithTests(ctx context.Context, rootDir string, testDir string, call configs.StaticModuleCall) (*configs.Config, hcl.Diagnostics) {
	l, err := c.init()
	if err != nil {
		return nil, hcl.Diagnostics{
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Config loader init failed",
				Detail:   fmt.Sprintf("Failed to initialise a config loader: %s", err),
			},
		}
	}
	return l.LoadConfigWithTests(ctx, rootDir, testDir, call)
}

// LoadConfigWithSnapshot implements Loader
func (c *lazyLoader) LoadConfigWithSnapshot(ctx context.Context, rootDir string, call configs.StaticModuleCall) (*configs.Config, *Snapshot, hcl.Diagnostics) {
	l, err := c.init()
	if err != nil {
		return nil, nil, hcl.Diagnostics{
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Config loader init failed",
				Detail:   fmt.Sprintf("Failed to initialise a config loader: %s", err),
			},
		}
	}
	return l.LoadConfigWithSnapshot(ctx, rootDir, call)
}

// IsRemoteModuleSource implements Loader
func (c *lazyLoader) IsRemoteModuleSource(path addrs.Module) bool {
	l, err := c.init()
	if err != nil {
		return false // Default as in the loader implementation
	}
	return l.IsRemoteModuleSource(path)
}

// ModuleSourceAddrs implements Loader
func (c *lazyLoader) ModuleSourceAddrs(path addrs.Module) addrs.ModuleSource {
	l, err := c.init()
	if err != nil {
		return nil // Default as in the loader implementation
	}
	return l.ModuleSourceAddrs(path)
}

// configs.Parser related methods

// LoadConfigDirUneval implements Loader
func (c *lazyLoader) LoadConfigDirUneval(path string, load configs.SelectiveLoader) (*configs.Module, hcl.Diagnostics) {
	l, err := c.init()
	if err != nil {
		return nil, hcl.Diagnostics{
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Config loader init failed",
				Detail:   fmt.Sprintf("Failed to initialise a config loader: %s", err),
			},
		}
	}
	return l.LoadConfigDirUneval(path, load)
}

// LoadConfigDir implements Loader
func (c *lazyLoader) LoadConfigDir(path string, call configs.StaticModuleCall) (*configs.Module, hcl.Diagnostics) {
	l, err := c.init()
	if err != nil {
		return nil, hcl.Diagnostics{
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Config loader init failed",
				Detail:   fmt.Sprintf("Failed to initialise a config loader: %s", err),
			},
		}
	}
	return l.LoadConfigDir(path, call)
}

// LoadHCLFile implements Loader
func (c *lazyLoader) LoadHCLFile(path string) (hcl.Body, hcl.Diagnostics) {
	l, err := c.init()
	if err != nil {
		return nil, hcl.Diagnostics{
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Config loader init failed",
				Detail:   fmt.Sprintf("Failed to initialise a config loader: %s", err),
			},
		}
	}
	return l.LoadHCLFile(path)
}

// LoadConfigDirSelective implements Loader
func (c *lazyLoader) LoadConfigDirSelective(path string, call configs.StaticModuleCall, load configs.SelectiveLoader) (*configs.Module, hcl.Diagnostics) {
	l, err := c.init()
	if err != nil {
		return nil, hcl.Diagnostics{
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Config loader init failed",
				Detail:   fmt.Sprintf("Failed to initialise a config loader: %s", err),
			},
		}
	}
	return l.LoadConfigDirSelective(path, call, load)
}

// LoadConfigDirWithTests implements Loader
func (c *lazyLoader) LoadConfigDirWithTests(path string, testDirectory string, call configs.StaticModuleCall) (*configs.Module, hcl.Diagnostics) {
	l, err := c.init()
	if err != nil {
		return nil, hcl.Diagnostics{
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Config loader init failed",
				Detail:   fmt.Sprintf("Failed to initialise a config loader: %s", err),
			},
		}
	}
	return l.LoadConfigDirWithTests(path, testDirectory, call)
}

// ForceFileSource allows to add synthetic additional source
// buffers to the config loader's cache of sources (as returned by
// configSources), which is useful when a command is directly parsing something
// from the command line that may produce diagnostics, so that diagnostic
// snippets can still be produced.
//
// This will try to load the underlying config loader but if there is an error it will turn
// the call into a no-op. (We presume that a caller will later call a different
// function that also initializes the config loader as a side effect, at which
// point those errors can be returned.)
func (c *lazyLoader) ForceFileSource(filename string, src []byte) {
	l, err := c.init()
	if err != nil {
		return // treated as no-op, since this is best-effort
	}
	l.ForceFileSource(filename, src)
}
