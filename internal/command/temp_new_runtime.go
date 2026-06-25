// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/apparentlymart/go-versions/versions"
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configload"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

func (m *Meta) NewRuntimeEnabled() bool {
	if !m.SystemCfg.AllowExperimentalFeatures {
		return false
	}
	optIn := os.Getenv("TOFU_X_EXPERIMENTAL_RUNTIME")
	return optIn != ""
}

func (m *Meta) StaticConfigInstance(ctx context.Context, root *configs.Module, modules eval.ExternalModules) (*eval.ConfigInstance, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	rootCall, moreDiags := m.rootModuleCall(ctx, root.SourceDir)
	diags = diags.Append(moreDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	workspace, err := m.Workspace(ctx)
	if err != nil {
		return nil, diags.Append(err)
	}

	rawInput := map[string]cty.Value{}
	varValue := rootCall.Variables()
	for _, variable := range root.Variables {
		val, hclDiags := varValue(variable)
		diags = diags.Append(hclDiags)
		if val == cty.NilVal {
			val = cty.DynamicVal
		}
		rawInput[variable.Name] = val
	}

	inputValues := exprs.ConstantValuer(cty.ObjectVal(rawInput))

	loader, err := m.initConfigLoader()
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to create the config loader",
			err.Error(),
		))
		diags = diags.Append(err)
		return nil, diags
	}

	owd := m.WorkingDir.OriginalWorkingDir()

	// The current working directory should always be absolute, whether we
	// just looked it up or whether we were relying on ContextMeta's
	// (possibly non-normalized) path.
	owd, err = filepath.Abs(owd)
	if err != nil {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  `Failed to get working directory`,
			Detail:   fmt.Sprintf(`The value for the original working directory cannot be determined due to a system error: %s`, err),
		})
		return nil, diags
	}

	if modules == nil {
		modules = &newRuntimeModules{
			loader: loader,
			root:   root,
		}
	}
	staticPlugins := eval.NewStaticPlugins()
	evalCtx := &eval.EvalContext{
		RootModuleDir:      root.SourceDir,
		OriginalWorkingDir: owd,
		Modules:            modules,
		Providers:          staticPlugins,
		Provisioners:       staticPlugins,
		Workspace:          workspace,
	}

	// The new config-loading system wants to work in terms of module source
	// addresses rather than raw local filenames, so we'll ask the
	// addrs package to parse the path we were given. We need to adjust
	// a little though, because this function was designed for parsing
	// the "source" argument in a module block, not a plain filepath.
	// We should add a function in package addrs that's actually intended for
	// turning arbitrary filesystem paths in to addrs.LocalSource in the long
	// run, but this will do for now.
	configDir := root.SourceDir
	if !filepath.IsAbs(configDir) {
		configDir = "." + string(filepath.Separator) + configDir
	}
	rootModuleSource, err := addrs.ParseModuleSource(configDir)
	if err != nil {
		diags = diags.Append(fmt.Errorf("invalid root module source address: %w", err))
		return nil, diags
	}

	configCall := &eval.ConfigCall{
		RootModuleSource:     rootModuleSource,
		InputValues:          inputValues,
		AllowImpureFunctions: false,
		EvalContext:          evalCtx,
	}
	configInst, moreDiags := eval.NewConfigInstance(ctx, configCall)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return nil, diags
	}
	return configInst, diags
}

// newRuntimeModules is an implementation of [eval.ExternalModules] that makes
// a best effort to shim to OpenTofu's current module loader, even though
// it works in some slightly-different terms than this new API expects.
type newRuntimeModules struct {
	loader *configload.Loader
	root   *configs.Module

	// configload.Loader is not concurrency-safe because it wraps
	// hclparse.Parser functionality that is not concurrency-safe, so we must
	// hold this lock whenever we're interacting with the loader object.
	mu sync.Mutex
}

var _ eval.ExternalModules = (*newRuntimeModules)(nil)

// ModuleConfig implements evalglue.ExternalModules.
func (n *newRuntimeModules) ModuleConfig(ctx context.Context, source addrs.ModuleSource, allowedVersions versions.Set, forCall *addrs.AbsModuleCall) (eval.UncompiledModule, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	if forCall == nil {
		return eval.PrepareTofu2024Module(source, n.root), diags
	}

	record, hclDiags := n.loader.ModuleLocalPath(ctx, &configs.ModuleRequest{
		Name:              forCall.Call.Name,
		Path:              forCall.Module.Module().Child(forCall.Call.Name),
		SourceAddr:        source,
		VersionConstraint: configs.VersionConstraint{},
		//SourceAddrRange hcl.Range
		//CallRange hcl.Range
		Parent: &configs.Config{
			Path: forCall.Module.Module(),
		},
	})
	diags = diags.Append(hclDiags)

	if diags.HasErrors() {
		return nil, diags
	}

	sourceDir := record.Dir

	log.Printf("[TRACE] backend/local: Loading module from %q from local path %q", source, sourceDir)

	n.mu.Lock()
	mod, hclDiags := n.loader.Parser().LoadConfigDirUneval(sourceDir, configs.SelectiveLoadAll)
	n.mu.Unlock()
	diags = diags.Append(hclDiags)
	if hclDiags.HasErrors() {
		return nil, diags
	}

	return eval.PrepareTofu2024Module(source, mod), diags
}
