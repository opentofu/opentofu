// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/provisioners"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
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
	Hooks        []Hook
	Parallelism  int
	Providers    map[addrs.Provider]providers.Factory
	Provisioners map[string]provisioners.Factory
	Encryption   encryption.Encryption

	UIInput UIInput
}

// ContextMeta is metadata about the running context. This is information
// that this package or structure cannot determine on its own but exposes
// into OpenTofu in various ways. This must be provided by the Context
// initializer.
type ContextMeta struct {
	Env string // Env is the state environment

	// OriginalWorkingDir is the working directory where the OpenTofu CLI
	// was run from, which may no longer actually be the current working
	// directory if the user included the -chdir=... option.
	//
	// If this string is empty then the original working directory is the same
	// as the current working directory.
	//
	// In most cases we should respect the user's override by ignoring this
	// path and just using the current working directory, but this is here
	// for some exceptional cases where the original working directory is
	// needed.
	OriginalWorkingDir string
}

// Context represents all the context that OpenTofu needs in order to
// perform operations on infrastructure. This structure is built using
// NewContext.
type Context struct {
	// meta captures some misc. information about the working directory where
	// we're taking these actions, and thus which should remain steady between
	// operations.
	meta *ContextMeta

	plugins *contextPlugins

	hooks   []Hook
	sh      *stopHook
	uiInput UIInput

	parallelSem         Semaphore
	l                   sync.Mutex // Lock acquired during any task
	providerInputConfig map[string]map[string]cty.Value
	runCond             *sync.Cond
	runContext          context.Context
	runContextCancel    context.CancelFunc

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
	hooks := make([]Hook, len(opts.Hooks)+1)
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

func (c *Context) Schemas(config *configs.Config, state *states.State) (*Schemas, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	ret, err := loadSchemas(config, state, c.plugins)
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
	if c.runContextCancel != nil {
		log.Printf("[WARN] tofu: run context exists, stopping")

		// Tell the hook we want to stop
		c.sh.Stop()

		// Stop the context
		c.runContextCancel()
		c.runContextCancel = nil
	}

	// Notify all of the hooks that we're stopping, in case they want to try
	// to flush in-memory state to disk before a subsequent hard kill.
	for _, hook := range c.hooks {
		hook.Stopping()
	}

	// Grab the condition var before we exit
	if cond := c.runCond; cond != nil {
		log.Printf("[INFO] tofu: waiting for graceful stop to complete")
		cond.Wait()
	}

	log.Printf("[WARN] tofu: stop complete")
}

func (c *Context) acquireRun(phase string) func() {
	// With the run lock held, grab the context lock to make changes
	// to the run context.
	c.l.Lock()
	defer c.l.Unlock()

	// Wait until we're no longer running
	for c.runCond != nil {
		c.runCond.Wait()
	}

	// Build our lock
	c.runCond = sync.NewCond(&c.l)

	// Create a new run context
	c.runContext, c.runContextCancel = context.WithCancel(context.Background())

	// Reset the stop hook so we're not stopped
	c.sh.Reset()

	return c.releaseRun
}

func (c *Context) releaseRun() {
	// Grab the context lock so that we can make modifications to fields
	c.l.Lock()
	defer c.l.Unlock()

	// End our run. We check if runContext is non-nil because it can be
	// set to nil if it was cancelled via Stop()
	if c.runContextCancel != nil {
		c.runContextCancel()
	}

	// Unlock all waiting our condition
	cond := c.runCond
	c.runCond = nil
	cond.Broadcast()

	// Unset the context
	c.runContext = nil
}

// watchStop immediately returns a `stop` and a `wait` chan after dispatching
// the watchStop goroutine. This will watch the runContext for cancellation and
// stop the providers accordingly.  When the watch is no longer needed, the
// `stop` chan should be closed before waiting on the `wait` chan.
// The `wait` chan is important, because without synchronizing with the end of
// the watchStop goroutine, the runContext may also be closed during the select
// incorrectly causing providers to be stopped. Even if the graph walk is done
// at that point, stopping a provider permanently cancels its StopContext which
// can cause later actions to fail.
func (c *Context) watchStop(walker *ContextGraphWalker) (chan struct{}, <-chan struct{}) {
	stop := make(chan struct{})
	wait := make(chan struct{})

	// get the runContext cancellation channel now, because releaseRun will
	// write to the runContext field.
	done := c.runContext.Done()

	panicHandler := logging.PanicHandlerWithTraceFn()
	go func() {
		defer panicHandler()

		defer close(wait)
		// Wait for a stop or completion
		select {
		case <-done:
			// done means the context was canceled, so we need to try and stop
			// providers.
		case <-stop:
			// our own stop channel was closed.
			return
		}

		// If we're here, we're stopped, trigger the call.
		log.Printf("[TRACE] Context: requesting providers and provisioners to gracefully stop")

		{
			// Copy the providers so that a misbehaved blocking Stop doesn't
			// completely hang OpenTofu.
			walker.providerLock.Lock()
			toStop := make([]providers.Interface, 0, len(walker.providerCache))
			for _, providerMap := range walker.providerCache {
				for _, provider := range providerMap {
					toStop = append(toStop, provider)
				}
			}
			defer walker.providerLock.Unlock()

			for _, p := range toStop {
				// We ignore the error for now since there isn't any reasonable
				// action to take if there is an error here, since the stop is still
				// advisory: OpenTofu will exit once the graph node completes.
				p.Stop()
			}
		}

		{
			// Call stop on all the provisioners
			walker.provisionerLock.Lock()
			ps := make([]provisioners.Interface, 0, len(walker.provisionerCache))
			for _, p := range walker.provisionerCache {
				ps = append(ps, p)
			}
			defer walker.provisionerLock.Unlock()

			for _, p := range ps {
				// We ignore the error for now since there isn't any reasonable
				// action to take if there is an error here, since the stop is still
				// advisory: OpenTofu will exit once the graph node completes.
				p.Stop()
			}
		}
	}()

	return stop, wait
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
