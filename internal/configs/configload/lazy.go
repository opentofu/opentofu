package configload

import (
	"context"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/configs"
)

type initializer interface {
	init() (Loader, hcl.Diagnostics)
}

type lazyLoader struct {
	cached Loader

	cfg *Config
}

func NewLazy(c *Config) Loader {
	return &lazyLoader{
		cfg: c,
	}
}

// TODO andrei add docs
func Initialize(l Loader) (Loader, error) {
	ll, ok := l.(*lazyLoader)
	if !ok {
		return l, nil
	}
	return ll.init()
}

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

func (c *lazyLoader) ImportSources(sources map[string][]byte) {
	l, err := c.init()
	if err != nil {
		return
	}
	l.ImportSources(sources)
}

func (c *lazyLoader) ImportSourcesFromSnapshot(snap *Snapshot) {
	l, err := c.init()
	if err != nil {
		return
	}
	l.ImportSourcesFromSnapshot(snap)
}

func (c *lazyLoader) IsConfigDir(path string) bool {
	l, err := c.init()
	if err != nil {
		return true // The same default with the previous version in Meta structure
	}
	return l.IsConfigDir(path)
}

func (c *lazyLoader) ModulesDir() string {
	l, err := c.init()
	if err != nil {
		return "" // TODO andrei - maybe we should panic here? If the init does not work, then it's a problem with the underlying loader init
	}
	return l.ModulesDir()
}

func (c *lazyLoader) RefreshModules() error {
	l, err := c.init()
	if err != nil {
		return err
	}
	return l.RefreshModules()
}

func (c *lazyLoader) Sources() map[string]*hcl.File {
	l, err := c.init()
	if err != nil {
		return nil
	}
	return l.Sources()
}

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

// configs.Parser related methods

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
