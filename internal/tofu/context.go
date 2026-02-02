// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"sync"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/engine"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/provisioners"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu/contract"
	"github.com/opentofu/opentofu/internal/tofu/graph"
	"github.com/opentofu/opentofu/internal/tofu/hooks"
	"github.com/opentofu/opentofu/internal/tofu/variables"
)

// InputMode defines what sort of input will be asked for when Input
// is called on Context.
type InputMode byte

const (
	// InputModeProvider asks for provider variables
	InputModeProvider InputMode = 1 << iota

	// InputModeStd is the standard operating mode and asks for both variables
	// and providers.
	InputModeStd = InputModeProvider
)

// ContextOpts are the user-configurable options to create a context with
// NewContext.
type ContextOpts struct {
	Meta         *ContextMeta
	Hooks        []hooks.Hook
	Parallelism  int
	Providers    map[addrs.Provider]providers.Factory
	Provisioners map[string]provisioners.Factory
	Encryption   encryption.Encryption

	UIInput UIInput
}

type ContextMeta contract.ContextMeta

// Context represents all the context that OpenTofu needs in order to
// perform operations on infrastructure. This structure is built using
// NewContext.
type Context struct {
	// meta captures some misc. information about the working directory where
	// we're taking these actions, and thus which should remain steady between
	// operations.
	meta *ContextMeta

	plugins *contextPlugins

	hooks   []hooks.Hook
	sh      *stopHook
	uiInput UIInput

	parallelSem         Semaphore
	l                   sync.Mutex // Lock acquired during any task
	providerInputConfig map[string]map[string]cty.Value

	runContext contract.Context

	encryption encryption.Encryption
}

// (additional methods on Context can be found in context_*.go files.)

// NewContext creates a new Context structure.
//
// Once a Context is created, the caller must not access or mutate any of
// the objects referenced (directly or indirectly) by the ContextOpts fields.
//
// If the returned diagnostics contains errors then the resulting context is
// invalid and must not be used.
func NewContext(opts *ContextOpts) (*Context, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	log.Printf("[TRACE] tofu.NewContext: starting")

	// Copy all the hooks and add our stop hook. We don't append directly
	// to the Config so that we're not modifying that in-place.
	sh := new(stopHook)
	hooks := make([]hooks.Hook, len(opts.Hooks)+1)
	copy(hooks, opts.Hooks)
	hooks[len(opts.Hooks)] = sh

	// Determine parallelism, default to 10. We do this both to limit
	// CPU pressure but also to have an extra guard against rate throttling
	// from providers.
	// We throw an error in case of negative parallelism
	par := opts.Parallelism
	if par < 0 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid parallelism value",
			fmt.Sprintf("The parallelism must be a positive value. Not %d.", par),
		))
		return nil, diags
	}

	if par == 0 {
		par = 10
	}

	plugins := newContextPlugins(opts.Providers, opts.Provisioners)

	log.Printf("[TRACE] tofu.NewContext: complete")

	return &Context{
		hooks:   hooks,
		meta:    opts.Meta,
		uiInput: opts.UIInput,

		plugins: plugins,

		parallelSem:         NewSemaphore(par),
		providerInputConfig: make(map[string]map[string]cty.Value),
		sh:                  sh,

		encryption: opts.Encryption,
	}, diags
}

func (c *Context) impl() (contract.Context, tfdiags.Diagnostics) {
	if os.Getenv("TOFU_X_EXPERIMENTAL_RUNTIME") != "" {
		return engine.NewContext(
			c.plugins.providerFactories,
			c.plugins.provisionerFactories,
		), nil
	}
	return graph.NewContext(&contract.ContextOpts{
		Meta:         (*contract.ContextMeta)(c.meta),
		Hooks:        c.hooks,
		Parallelism:  10, // TODO cam72cam
		Providers:    c.plugins.providerFactories,
		Provisioners: c.plugins.provisionerFactories,
		Encryption:   c.encryption,
	})
}

func (c *Context) Schemas(ctx context.Context, config *configs.Config, state *states.State) (*Schemas, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	ret, err := loadSchemas(ctx, config, state, c.plugins)
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to load plugin schemas",
			fmt.Sprintf("Error while loading schemas for plugin components: %s.", err),
		))
		return nil, diags
	}
	return ret, diags
}

type ContextGraphOpts struct {
	// If true, validates the graph structure (checks for cycles).
	Validate bool

	// Legacy graphs only: won't prune the graph
	Verbose bool
}

// Stop stops the running task.
//
// Stop will block until the task completes.
func (c *Context) Stop() {
	log.Printf("[WARN] tofu: Stop called, initiating interrupt sequence")

	c.l.Lock()
	defer c.l.Unlock()

	// If we're running, then stop
	if c.runContext != nil {
		log.Printf("[WARN] tofu: run context exists, stopping")

		// Tell the hook we want to stop
		c.sh.Stop()

		// Stop the context
		c.runContext.Stop()

	}

	// Notify all of the hooks that we're stopping, in case they want to try
	// to flush in-memory state to disk before a subsequent hard kill.
	for _, hook := range c.hooks {
		hook.Stopping()
	}

}

func (c *Context) acquireRun(phase string) (contract.Context, func(), tfdiags.Diagnostics) {
	// With the run lock held, grab the context lock to make changes
	// to the run context.
	c.l.Lock()
	defer c.l.Unlock()

	if c.runContext != nil {
		panic("Invalid context state")
	}

	impl, diags := c.impl()
	if diags.HasErrors() {
		return nil, nil, diags
	}

	c.runContext = impl

	return c.runContext, c.releaseRun, diags
}

func (c *Context) releaseRun() {
	// Grab the context lock so that we can make modifications to fields
	c.l.Lock()
	defer c.l.Unlock()

	// Unset the context
	c.runContext = nil
}

// checkConfigDependencies checks whether the receiving context is able to
// support the given configuration, returning error diagnostics if not.
//
// Currently this function checks whether the current OpenTofu CLI version
// matches the version requirements of all of the modules, and whether our
// plugin library contains all of the plugin names/addresses needed.
//
// This function does *not* check that external modules are installed (that's
// the responsibility of the configuration loader) and doesn't check that the
// plugins are of suitable versions to match any version constraints (which is
// the responsibility of the code which installed the plugins and then
// constructed the Providers/Provisioners maps passed in to NewContext).
//
// In most cases we should typically catch the problems this function detects
// before we reach this point, but this function can come into play in some
// unusual cases outside of the main workflow, and can avoid some
// potentially-more-confusing errors from later operations.
func (c *Context) checkConfigDependencies(config *configs.Config) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	// This checks the OpenTofu CLI version constraints specified in all of
	// the modules.
	diags = diags.Append(CheckCoreVersionRequirements(config))

	// We only check that we have a factory for each required provider, and
	// assume the caller already assured that any separately-installed
	// plugins are of a suitable version, match expected checksums, etc.
	providerReqs, _, hclDiags := config.ProviderRequirements()
	diags = diags.Append(hclDiags)
	if hclDiags.HasErrors() {
		return diags
	}
	for providerAddr := range providerReqs {
		if !c.plugins.HasProvider(providerAddr) {
			if !providerAddr.IsBuiltIn() {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Missing required provider",
					fmt.Sprintf(
						"This configuration requires provider %s, but that provider isn't available. You may be able to install it automatically by running:\n  tofu init",
						providerAddr,
					),
				))
			} else {
				// Built-in providers can never be installed by "tofu init",
				// so no point in confusing the user by suggesting that.
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Missing required provider",
					fmt.Sprintf(
						"This configuration requires built-in provider %s, but that provider isn't available in this OpenTofu version.",
						providerAddr,
					),
				))
			}
		}
	}

	// Our handling of provisioners is much less sophisticated than providers
	// because they are in many ways a legacy system. We need to go hunting
	// for them more directly in the configuration.
	config.DeepEach(func(modCfg *configs.Config) {
		if modCfg == nil || modCfg.Module == nil {
			return // should not happen, but we'll be robust
		}
		for _, rc := range modCfg.Module.ManagedResources {
			if rc.Managed == nil {
				continue // should not happen, but we'll be robust
			}
			for _, pc := range rc.Managed.Provisioners {
				if !c.plugins.HasProvisioner(pc.Type) {
					// This is not a very high-quality error, because really
					// the caller of tofu.NewContext should've already
					// done equivalent checks when doing plugin discovery.
					// This is just to make sure we return a predictable
					// error in a central place, rather than failing somewhere
					// later in the non-deterministically-ordered graph walk.
					diags = diags.Append(tfdiags.Sourceless(
						tfdiags.Error,
						"Missing required provisioner plugin",
						fmt.Sprintf(
							"This configuration requires provisioner plugin %q, which isn't available. If you're intending to use an external provisioner plugin, you must install it manually into one of the plugin search directories before running OpenTofu.",
							pc.Type,
						),
					))
				}
			}
		}
	})

	// Because we were doing a lot of map iteration above, and we're only
	// generating sourceless diagnostics anyway, our diagnostics will not be
	// in a deterministic order. To ensure stable output when there are
	// multiple errors to report, we'll sort these particular diagnostics
	// so they are at least always consistent alone. This ordering is
	// arbitrary and not a compatibility constraint.
	sort.Slice(diags, func(i, j int) bool {
		// Because these are sourceless diagnostics and we know they are all
		// errors, we know they'll only differ in their description fields.
		descI := diags[i].Description()
		descJ := diags[j].Description()
		switch {
		case descI.Summary != descJ.Summary:
			return descI.Summary < descJ.Summary
		default:
			return descI.Detail < descJ.Detail
		}
	})

	return diags
}

// checkInputVariables ensures that the caller provided an InputValue
// definition for each root module variable declared in the configuration.
// The caller must provide an InputVariables with keys exactly matching
// the declared variables, though some of them may be marked explicitly
// unset by their values being cty.NilVal.
//
// This doesn't perform any type checking, default value substitution, or
// validation checks. Those are all handled during a graph walk when we
// visit the graph nodes representing each root variable.
//
// The set of values is considered valid only if the returned diagnostics
// does not contain errors. A valid set of values may still produce warnings,
// which should be returned to the user.
func checkInputVariables(vcs map[string]*configs.Variable, vs variables.InputValues) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	for name := range vcs {
		_, isSet := vs[name]
		if !isSet {
			// Always an error, since the caller should have produced an
			// item with Value: cty.NilVal to be explicit that it offered
			// an opportunity to set this variable.
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Unassigned variable",
				fmt.Sprintf("The input variable %q has not been assigned a value. This is a bug in OpenTofu; please report it in a GitHub issue.", name),
			))
			continue
		}
	}

	// Check for any variables that are assigned without being configured.
	// This is always an implementation error in the caller, because we
	// expect undefined variables to be caught during context construction
	// where there is better context to report it well.
	for name := range vs {
		if _, defined := vcs[name]; !defined {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Value assigned to undeclared variable",
				fmt.Sprintf("A value was assigned to an undeclared input variable %q.", name),
			))
		}
	}

	return diags
}
