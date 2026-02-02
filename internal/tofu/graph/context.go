// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package graph

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/provisioners"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu/contract"
	"github.com/opentofu/opentofu/internal/tofu/hooks"
)

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

	hooks []hooks.Hook
	sh    *stopHook

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
func NewContext(opts *contract.ContextOpts) (contract.Context, tfdiags.Diagnostics) {
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
		hooks: hooks,
		meta:  (*ContextMeta)(opts.Meta),

		plugins: plugins,

		parallelSem:         NewSemaphore(par),
		providerInputConfig: make(map[string]map[string]cty.Value),
		sh:                  sh,

		encryption: opts.Encryption,
	}, diags
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
				// The providers.Interface API contract requires that the
				// context passed to Stop is never canceled and has no deadline.
				_ = p.Stop(context.WithoutCancel(context.TODO()))
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
				_ = p.Stop()
			}
		}
	}()

	return stop, wait
}
