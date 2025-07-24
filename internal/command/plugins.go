// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/kardianos/osext"
	"go.rpcplugin.org/rpcplugin"
	"google.golang.org/grpc"

	fileprovisioner "github.com/opentofu/opentofu/internal/builtin/provisioners/file"
	localexec "github.com/opentofu/opentofu/internal/builtin/provisioners/local-exec"
	remoteexec "github.com/opentofu/opentofu/internal/builtin/provisioners/remote-exec"
	tfplugin "github.com/opentofu/opentofu/internal/plugin"
	"github.com/opentofu/opentofu/internal/plugin/discovery"
	"github.com/opentofu/opentofu/internal/provisioners"
)

// NOTE WELL: The logic in this file is primarily about plugin types OTHER THAN
// providers, which use an older set of approaches implemented here.
//
// The provider-related functions live primarily in meta_providers.go, and
// lean on some different underlying mechanisms in order to support automatic
// installation and a hierarchical addressing namespace, neither of which
// are supported for other plugin types.

// store the user-supplied path for plugin discovery
func (m *Meta) storePluginPath(pluginPath []string) error {
	if len(pluginPath) == 0 {
		return nil
	}

	m.fixupMissingWorkingDir()

	// remove the plugin dir record if the path was set to an empty string
	if len(pluginPath) == 1 && (pluginPath[0] == "") {
		return m.WorkingDir.SetForcedPluginDirs(nil)
	}

	return m.WorkingDir.SetForcedPluginDirs(pluginPath)
}

// Load the user-defined plugin search path into Meta.pluginPath if the file
// exists.
func (m *Meta) loadPluginPath() ([]string, error) {
	m.fixupMissingWorkingDir()
	return m.WorkingDir.ForcedPluginDirs()
}

// the default location for automatically installed plugins
func (m *Meta) pluginDir() string {
	return filepath.Join(m.DataDir(), "plugins", fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH))
}

// pluginDirs return a list of directories to search for plugins.
//
// Earlier entries in this slice get priority over later when multiple copies
// of the same plugin version are found, but newer versions always override
// older versions where both satisfy the provider version constraints.
func (m *Meta) pluginDirs(includeAutoInstalled bool) []string {
	// user defined paths take precedence
	if len(m.pluginPath) > 0 {
		return m.pluginPath
	}

	// When searching the following directories, earlier entries get precedence
	// if the same plugin version is found twice, but newer versions will
	// always get preference below regardless of where they are coming from.
	// TODO: Add auto-install dir, default vendor dir and optional override
	// vendor dir(s).
	dirs := []string{"."}

	// Look in the same directory as the OpenTofu executable.
	// If found, this replaces what we found in the config path.
	exePath, err := osext.Executable()
	if err != nil {
		log.Printf("[ERROR] Error discovering exe directory: %s", err)
	} else {
		dirs = append(dirs, filepath.Dir(exePath))
	}

	// add the user vendor directory
	dirs = append(dirs, DefaultPluginVendorDir)

	if includeAutoInstalled {
		dirs = append(dirs, m.pluginDir())
	}
	dirs = append(dirs, m.GlobalPluginDirs...)

	return dirs
}

func (m *Meta) provisionerFactories() map[string]provisioners.Factory {
	dirs := m.pluginDirs(true)
	plugins := discovery.FindPlugins("provisioner", dirs)
	plugins, _ = plugins.ValidateVersions()

	// For now our goal is to just find the latest version of each plugin
	// we have on the system. All provisioners should be at version 0.0.0
	// currently, so there should actually only be one instance of each plugin
	// name here, even though the discovery interface forces us to pretend
	// that might not be true.

	factories := make(map[string]provisioners.Factory)

	// Wire up the internal provisioners first. These might be overridden
	// by discovered provisioners below.
	for name, factory := range internalProvisionerFactories() {
		factories[name] = factory
	}

	byName := plugins.ByName()
	for name, metas := range byName {
		// Since we validated versions above and we partitioned the sets
		// by name, we're guaranteed that the metas in our set all have
		// valid versions and that there's at least one meta.
		newest := metas.Newest()

		factories[name] = provisionerFactory(newest)
	}

	return factories
}

func provisionerFactory(meta discovery.PluginMeta) provisioners.Factory {
	return func() (provisioners.Interface, error) {
		plugin, err := rpcplugin.New(context.Background(), &rpcplugin.ClientConfig{
			Cmd:       exec.Command(meta.Path),
			Handshake: tfplugin.Handshake,
			ProtoVersions: map[int]rpcplugin.ClientVersion{
				5: rpcplugin.ClientVersionFunc(func(ctx context.Context, conn *grpc.ClientConn) (interface{}, error) {
					return tfplugin.NewGRPCProvisioner(ctx, conn), nil
				}),
			},
			// FIXME: Set Stderr to the write end of a pipe connected to
			// something that interprets the stderr stream as a series
			// of log lines similar to what go-plugin does. For now we
			// just completely discard plugin stderr.
		})
		if err != nil {
			return nil, err
		}
		_, clientI, err := plugin.Client(context.Background())
		if err != nil {
			return nil, err
		}

		client := clientI.(*tfplugin.GRPCProvisioner)
		client.PluginClient = plugin
		return client, nil
	}
}

func internalProvisionerFactories() map[string]provisioners.Factory {
	return map[string]provisioners.Factory{
		"file":        provisioners.FactoryFixed(fileprovisioner.New()),
		"local-exec":  provisioners.FactoryFixed(localexec.New()),
		"remote-exec": provisioners.FactoryFixed(remoteexec.New()),
	}
}
