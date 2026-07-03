// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package initwd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/apparentlymart/go-versions/versions"
	version "github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configload"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/modsdir"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func (i *ModuleInstaller) installDescendentModulesNewRuntime(ctx context.Context, rootDir string, manifest modsdir.Manifest, installWalker configs.ModuleWalker, installErrsOnly bool) (*configs.Config, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// When attempting to initialize the current directory with a module
	// source, some use cases may want to ignore configuration errors from the
	// building of the entire configuration structure, but we still need to
	// capture installation errors. Because the actual module installation
	// happens in the ModuleWalkFunc callback while building the config, we
	// need to create a closure to capture the installation diagnostics
	// separately.
	var instDiags hcl.Diagnostics
	walker := installWalker
	if installErrsOnly {
		walker = configs.ModuleWalkerFunc(func(ctx context.Context, req *configs.ModuleRequest) (*configs.Module, *version.Version, hcl.Diagnostics) {
			mod, version, diags := installWalker.LoadModule(ctx, req)
			instDiags = instDiags.Extend(diags)
			return mod, version, diags
		})
	}

	root, hclDiags := i.loader.LoadConfigDirUneval(rootDir, configs.SelectiveLoadAll)
	diags = diags.Append(hclDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	configInst, moreDiags := i.ConfigInstance(ctx, root, &newRuntimeModules{
		loader: i.loader,
		walker: walker,

		root:    root,
		rootDir: rootDir,
	})
	diags = diags.Append(moreDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	moreDiags = configInst.StaticCheck(ctx)
	diags = diags.Append(moreDiags)

	if installErrsOnly {
		// We can't continue if there was an error during installation, but
		// return all diagnostics in case there happens to be anything else
		// useful when debugging the problem. Any instDiags will be included in
		// diags already.
		if instDiags.HasErrors() {
			return nil, diags
		}

		// If there are any errors here, they must be only from building the
		// config structures. We don't want to block initialization at this
		// point, so convert these into warnings. Any actual errors in the
		// configuration will be raised as soon as the config is loaded again.
		// We continue below because writing the manifest is required to finish
		// module installation.
		diags = tfdiags.OverrideAll(diags, tfdiags.Warning, nil)
	}

	err := manifest.WriteSnapshotToDir(i.modsDir)
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to update module manifest",
			fmt.Sprintf("Unable to write the module manifest file: %s", err),
		))
	}

	return nil, diags
}

// newRuntimeModules is an implementation of [eval.ExternalModules] that makes
// a best effort to shim to OpenTofu's current module loader, even though
// it works in some slightly-different terms than this new API expects.
type newRuntimeModules struct {
	loader configload.Loader
	walker configs.ModuleWalker

	root    *configs.Module
	rootDir string

	// configload.Loader is not concurrency-safe because it wraps
	// hclparse.Parser functionality that is not concurrency-safe, so we must
	// hold this lock whenever we're interacting with the loader object.
	mu sync.Mutex
}

var _ eval.ExternalModules = (*newRuntimeModules)(nil)

// ModuleConfig implements eval.ExternalModules.
func (n *newRuntimeModules) ModuleConfig(ctx context.Context, source addrs.ModuleSource, allowedVersions versions.Set, forCall *addrs.AbsModuleCall) (eval.UncompiledModule, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	if forCall == nil {
		// Root Module
		return eval.PrepareTofu2024Module(source, n.root), diags
	}

	if remoteSource, ok := source.(addrs.ModuleSourceRemote); ok {
		str := remoteSource.String()
		if strings.HasPrefix(str, "file://") {
			// This is truly terrible and should not ever be released...
			abs, _ := filepath.Abs(n.rootDir)

			sourceDir := strings.TrimPrefix(str, "file://")

			rel, err := filepath.Rel(abs, sourceDir)
			if err != nil {
				panic(err)
			}
			source = addrs.ModuleSourceLocal(n.rootDir + string(filepath.Separator) + rel)
		}
	}

	n.mu.Lock()
	mod, _, modDiags := n.walker.LoadModule(ctx, &configs.ModuleRequest{
		Name:       forCall.Call.Name,
		Path:       forCall.Module.Module().Child(forCall.Call.Name),
		SourceAddr: source,
		VersionConstraint: configs.VersionConstraint{
			RequiredSet: allowedVersions,
		},
		//SourceAddrRange hcl.Range
		//CallRange hcl.Range
		Parent: &configs.Config{
			Path: forCall.Module.Module(),
		},
	})
	n.mu.Unlock()
	diags = diags.Append(modDiags)

	return eval.PrepareTofu2024Module(source, mod), diags
}
