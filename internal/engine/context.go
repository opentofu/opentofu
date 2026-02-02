package engine

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/apparentlymart/go-versions/versions"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configload"
	"github.com/opentofu/opentofu/internal/engine/planning"
	"github.com/opentofu/opentofu/internal/engine/plugins"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/provisioners"
	"github.com/opentofu/opentofu/internal/refactoring"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu/contract"
	"github.com/opentofu/opentofu/internal/tofu/importing"
	"github.com/opentofu/opentofu/internal/tofu/variables"
	"github.com/zclconf/go-cty/cty"
)

type Context struct {
	plugins plugins.Plugins
}

func NewContext(
	providers map[addrs.Provider]providers.Factory,
	provisioners map[string]provisioners.Factory,
) *Context {
	return &Context{
		plugins: plugins.NewRuntimePlugins(providers, provisioners),
	}
}

func (c *Context) Validate(ctx context.Context, config *configs.Config, varValues variables.InputValues, importTargets []*importing.ImportTarget) tfdiags.Diagnostics {
	panic("not implemented") // TODO: Implement
}

func (c *Context) Plan(ctx context.Context, config *configs.Config, prevRunState *states.State, moveStmts []refactoring.MoveStatement, moveResults refactoring.MoveResults, opts *contract.PlanOpts) (*plans.Plan, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	configDir := config.Module.SourceDir
	if !filepath.IsAbs(configDir) {
		configDir = "." + string(filepath.Separator) + configDir
	} else {
		basepath, err := os.Getwd()

		configDir, err = filepath.Rel(basepath, configDir)
		if err != nil {
			panic(err)
		}
	}

	rootModuleSource, err := addrs.ParseModuleSource(configDir)
	if err != nil {
		return nil, diags.Append(err)
	}

	loader, _ := configload.NewLoader(&configload.Config{})

	evalCtx := &eval.EvalContext{
		RootModuleDir:      configDir,
		OriginalWorkingDir: configDir,
		Modules: &newRuntimeModules{
			loader: loader,
		},
		Providers:    c.plugins,
		Provisioners: c.plugins,
	}

	rawInput := map[string]cty.Value{}
	for name, value := range opts.SetVariables {
		if !value.Value.IsNull() {
			rawInput[name] = value.Value
		}
	}

	inputValues := exprs.ConstantValuer(cty.ObjectVal(rawInput))

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

	return planning.PlanChanges(ctx, prevRunState, configInst, c.plugins)
}

func (c *Context) Apply(ctx context.Context, plan *plans.Plan, config *configs.Config, setVariables variables.InputValues) (*states.State, tfdiags.Diagnostics) {
	panic("not implemented") // TODO: Implement
}

func (c *Context) Import(ctx context.Context, config *configs.Config, prevRunState *states.State, opts *contract.ImportOpts) (*states.State, tfdiags.Diagnostics) {
	panic("not implemented") // TODO: Implement
}

func (c *Context) Eval(ctx context.Context, config *configs.Config, state *states.State, moduleAddr addrs.ModuleInstance, variables variables.InputValues) (*lang.Scope, tfdiags.Diagnostics) {
	panic("not implemented") // TODO: Implement
}

func (c *Context) Stop() {
	panic("not implemented") // TODO: Implement
}

// newRuntimeModules is an implementation of [eval.ExternalModules] that makes
// a best effort to shim to OpenTofu's current module loader, even though
// it works in some slightly-different terms than this new API expects.
type newRuntimeModules struct {
	loader *configload.Loader
}

var _ eval.ExternalModules = (*newRuntimeModules)(nil)

// ModuleConfig implements evalglue.ExternalModules.
func (n *newRuntimeModules) ModuleConfig(ctx context.Context, source addrs.ModuleSource, allowedVersions versions.Set, forCall *addrs.AbsModuleCall) (eval.UncompiledModule, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	var sourceDir string
	switch source := source.(type) {
	case addrs.ModuleSourceLocal:
		sourceDir = filepath.Clean(filepath.FromSlash(string(source)))
	default:
		// For this early stub implementation we only support local source
		// addresses. We'll expand this later but that'll require this codepath
		// to have access to the information about what's in the module cache
		// directory at ".terraform/modules", which we've not arranged for yet.
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"New runtime codepath only supports local module sources",
			fmt.Sprintf("Cannot load %q, because our temporary codepath for the new language runtime only supports local module sources for now.", source),
		))
		return nil, diags
	}
	log.Printf("[TRACE] backend/local: Loading module from %q from local path %q", source, sourceDir)

	mod, hclDiags := n.loader.Parser().LoadConfigDirUneval(sourceDir, configs.SelectiveLoadAll)
	diags = diags.Append(hclDiags)
	if hclDiags.HasErrors() {
		return nil, diags
	}

	return eval.PrepareTofu2024Module(source, mod), diags
}
