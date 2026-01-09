// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/checks"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/plugins"
	"github.com/opentofu/opentofu/internal/refactoring"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// BuiltinEvalContext is an EvalContext implementation that is used by
// OpenTofu by default.
type BuiltinEvalContext struct {
	// StopContext is the context used to track whether we're complete
	StopContext context.Context

	// PathValue is the Path that this context is operating within.
	PathValue addrs.ModuleInstance

	// pathSet indicates that this context was explicitly created for a
	// specific path, and can be safely used for evaluation. This lets us
	// differentiate between PathValue being unset, and the zero value which is
	// equivalent to RootModuleInstance.  Path and Evaluation methods will
	// panic if this is not set.
	pathSet bool

	// Evaluator is used for evaluating expressions within the scope of this
	// eval context.
	Evaluator *Evaluator

	VariableValuesLock *sync.Mutex
	// VariableValues contains the variable values across all modules. This
	// structure is shared across the entire containing context, and so it
	// may be accessed only when holding VariableValuesLock.
	// The keys of the first level of VariableValues are the string
	// representations of addrs.ModuleInstance values. The second-level keys
	// are variable names within each module instance.
	VariableValues map[string]map[string]cty.Value

	// Plugins is a library of plugin components (providers and provisioners)
	// available for use during a graph walk.
	Plugins *pluginsManager

	Hooks      []Hook
	InputValue UIInput

	ProviderLock        *sync.Mutex
	ProviderInputConfig map[string]map[string]cty.Value

	ChangesValue          *plans.ChangesSync
	StateValue            *states.SyncState
	ChecksValue           *checks.State
	RefreshStateValue     *states.SyncState
	PrevRunStateValue     *states.SyncState
	InstanceExpanderValue *instances.Expander
	MoveResultsValue      refactoring.MoveResults
	ImportResolverValue   *ImportResolver
	Encryption            encryption.Encryption
}

// BuiltinEvalContext implements EvalContext
var _ EvalContext = (*BuiltinEvalContext)(nil)

func (c *BuiltinEvalContext) WithPath(path addrs.ModuleInstance) EvalContext {
	newEvalCtx := *c
	newEvalCtx.pathSet = true
	newEvalCtx.PathValue = path
	return &newEvalCtx
}

func (c *BuiltinEvalContext) Stopped() <-chan struct{} {
	// This can happen during tests. During tests, we just block forever.
	if c.StopContext == nil {
		return nil
	}

	return c.StopContext.Done()
}

func (c *BuiltinEvalContext) Hook(fn func(Hook) (HookAction, error)) error {
	for _, h := range c.Hooks {
		action, err := fn(h)
		if err != nil {
			return err
		}

		switch action {
		case HookActionContinue:
			continue
		case HookActionHalt:
			// Return an early exit error to trigger an early exit
			log.Printf("[WARN] Early exit triggered by hook: %T", h)
			return nil
		}
	}

	return nil
}

func (c *BuiltinEvalContext) Input() UIInput {
	return c.InputValue
}

func (c *BuiltinEvalContext) ProviderInput(_ context.Context, pc addrs.AbsProviderConfig) map[string]cty.Value {
	c.ProviderLock.Lock()
	defer c.ProviderLock.Unlock()

	if !c.Path().IsForModule(pc.Module) {
		// This indicates incorrect use of InitProvider: it should be used
		// only from the module that the provider configuration belongs to.
		panic(fmt.Sprintf("%s initialized by wrong module %s", pc, c.Path()))
	}

	if !c.Path().IsRoot() {
		// Only root module provider configurations can have input.
		return nil
	}

	return c.ProviderInputConfig[pc.String()]
}

func (c *BuiltinEvalContext) SetProviderInput(_ context.Context, pc addrs.AbsProviderConfig, vals map[string]cty.Value) {
	absProvider := pc
	if !pc.Module.IsRoot() {
		// Only root module provider configurations can have input.
		log.Printf("[WARN] BuiltinEvalContext: attempt to SetProviderInput for non-root module")
		return
	}

	// Save the configuration
	c.ProviderLock.Lock()
	c.ProviderInputConfig[absProvider.String()] = vals
	c.ProviderLock.Unlock()
}

func (c *BuiltinEvalContext) Providers() plugins.ProviderManager {
	return c.Plugins.providers
}

func (c *BuiltinEvalContext) Provisioners() plugins.ProvisionerManager {
	return c.Plugins.provisioners
}

func (c *BuiltinEvalContext) EvaluateBlock(ctx context.Context, body hcl.Body, schema *configschema.Block, self addrs.Referenceable, keyData InstanceKeyEvalData) (cty.Value, hcl.Body, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	scope := c.EvaluationScope(self, nil, keyData)
	body, evalDiags := scope.ExpandBlock(ctx, body, schema)
	diags = diags.Append(evalDiags)
	val, evalDiags := scope.EvalBlock(ctx, body, schema)
	diags = diags.Append(evalDiags)
	return val, body, diags
}

func (c *BuiltinEvalContext) EvaluateExpr(ctx context.Context, expr hcl.Expression, wantType cty.Type, self addrs.Referenceable) (cty.Value, tfdiags.Diagnostics) {
	scope := c.EvaluationScope(self, nil, EvalDataForNoInstanceKey)
	return scope.EvalExpr(ctx, expr, wantType)
}

func (c *BuiltinEvalContext) EvaluateReplaceTriggeredBy(ctx context.Context, expr hcl.Expression, repData instances.RepetitionData) (*addrs.Reference, bool, tfdiags.Diagnostics) {

	// get the reference to lookup changes in the plan
	ref, diags := evalReplaceTriggeredByExpr(expr, repData)
	if diags.HasErrors() {
		return nil, false, diags
	}

	var changes []*plans.ResourceInstanceChangeSrc
	// store the address once we get it for validation
	var resourceAddr addrs.Resource

	// The reference is either a resource or resource instance
	switch sub := ref.Subject.(type) {
	case addrs.Resource:
		resourceAddr = sub
		rc := sub.Absolute(c.Path())
		changes = c.Changes().GetChangesForAbsResource(rc)
	case addrs.ResourceInstance:
		resourceAddr = sub.ContainingResource()
		rc := sub.Absolute(c.Path())
		change := c.Changes().GetResourceInstanceChange(rc, states.CurrentGen)
		if change != nil {
			// we'll generate an error below if there was no change
			changes = append(changes, change)
		}
	}

	// Do some validation to make sure we are expecting a change at all
	cfg := c.Evaluator.Config.DescendentForInstance(c.Path())
	resCfg := cfg.Module.ResourceByAddr(resourceAddr)
	if resCfg == nil {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  `Reference to undeclared resource`,
			Detail:   fmt.Sprintf(`A resource %s has not been declared in %s`, ref.Subject, moduleDisplayAddr(c.Path())),
			Subject:  expr.Range().Ptr(),
		})
		return nil, false, diags
	}

	if len(changes) == 0 {
		// If the resource is valid there should always be at least one change.
		diags = diags.Append(fmt.Errorf("no change found for %s in %s", ref.Subject, moduleDisplayAddr(c.Path())))
		return nil, false, diags
	}

	// If we don't have a traversal beyond the resource, then we can just look
	// for any change.
	if len(ref.Remaining) == 0 {
		for _, c := range changes {
			switch c.ChangeSrc.Action {
			// Only immediate changes to the resource will trigger replacement.
			case plans.Update, plans.DeleteThenCreate, plans.CreateThenDelete:
				return ref, true, diags
			}
		}

		// no change triggered
		return nil, false, diags
	}

	// This must be an instances to have a remaining traversal, which means a
	// single change.
	change := changes[0]

	// Make sure the change is actionable. A create or delete action will have
	// a change in value, but are not valid for our purposes here.
	switch change.ChangeSrc.Action {
	case plans.Update, plans.DeleteThenCreate, plans.CreateThenDelete:
		// OK
	default:
		return nil, false, diags
	}

	// Since we have a traversal after the resource reference, we will need to
	// decode the changes, which means we need a schema.
	providerAddr := change.ProviderAddr
	schema := c.Providers().GetProviderSchema(ctx, providerAddr.Provider)
	if schema.Diagnostics.HasErrors() {
		diags = diags.Append(schema.Diagnostics)
		return nil, false, diags
	}

	resAddr := change.Addr.ContainingResource().Resource
	resSchema, _ := schema.SchemaForResourceType(resAddr.Mode, resAddr.Type)
	ty := resSchema.ImpliedType()

	before, err := change.ChangeSrc.Before.Decode(ty)
	if err != nil {
		diags = diags.Append(err)
		return nil, false, diags
	}

	after, err := change.ChangeSrc.After.Decode(ty)
	if err != nil {
		diags = diags.Append(err)
		return nil, false, diags
	}

	path := traversalToPath(ref.Remaining)
	attrBefore, _ := path.Apply(before)
	attrAfter, _ := path.Apply(after)

	if attrBefore == cty.NilVal || attrAfter == cty.NilVal {
		replace := attrBefore != attrAfter
		return ref, replace, diags
	}

	replace := !attrBefore.RawEquals(attrAfter)

	return ref, replace, diags
}

func (c *BuiltinEvalContext) EvaluationScope(self addrs.Referenceable, source addrs.Referenceable, keyData InstanceKeyEvalData) *lang.Scope {
	if !c.pathSet {
		panic("context path not set")
	}
	data := &evaluationStateData{
		Evaluator:       c.Evaluator,
		ModulePath:      c.PathValue,
		InstanceKeyData: keyData,
		Operation:       c.Evaluator.Operation,
	}

	// ctx.PathValue is the path of the module that contains whatever
	// expression the caller will be trying to evaluate, so this will
	// activate only the experiments from that particular module, to
	// be consistent with how experiment checking in the "configs"
	// package itself works. The nil check here is for robustness in
	// incompletely-mocked testing situations; mc should never be nil in
	// real situations.
	mc := c.Evaluator.Config.DescendentForInstance(c.PathValue)

	if mc == nil || mc.Module.ProviderRequirements == nil {
		return c.Evaluator.Scope(data, self, source, nil)
	}

	scope := c.Evaluator.Scope(data, self, source, func(ctx context.Context, pf addrs.ProviderFunction, rng tfdiags.SourceRange) (*function.Function, tfdiags.Diagnostics) {
		providedBy, ok := c.Plugins.providerFunctionTracker.Lookup(c.PathValue.Module(), pf)
		if !ok {
			// This should not be possible if references are tracked correctly
			return nil, tfdiags.Diagnostics{}.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "BUG: Uninitialized function provider",
				Detail:   fmt.Sprintf("Provider function %q has not been tracked properly", pf),
				Subject:  rng.ToHCL().Ptr(),
			})
		}

		var providerKey addrs.InstanceKey
		if providedBy.KeyExpression != nil && c.Evaluator.Operation != walkValidate {
			moduleInstanceForKey := c.PathValue[:len(providedBy.KeyModule)]
			if !moduleInstanceForKey.IsForModule(providedBy.KeyModule) {
				panic(fmt.Sprintf("Invalid module key expression location %s in function %s", providedBy.KeyModule, pf.String()))
			}

			var keyDiags tfdiags.Diagnostics
			providerKey, keyDiags = resolveProviderModuleInstance(ctx, c, providedBy.KeyExpression, moduleInstanceForKey, c.PathValue.String()+" "+pf.String())
			if keyDiags.HasErrors() {
				return nil, keyDiags
			}
		}

		provider := providedBy.Instance(providerKey)

		if provider == nil {
			// This should not be possible if references are tracked correctly
			return nil, tfdiags.Diagnostics{}.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Uninitialized function provider",
				Detail:   fmt.Sprintf("Provider %q has not yet been initialized", providedBy.Provider.String()),
				Subject:  rng.ToHCL().Ptr(),
			})
		}

		return evalContextProviderFunction(ctx, provider, c.Evaluator.Operation, pf, rng)
	})
	scope.SetActiveExperiments(mc.Module.ActiveExperiments)

	return scope
}

func (c *BuiltinEvalContext) Path() addrs.ModuleInstance {
	if !c.pathSet {
		panic("context path not set")
	}
	return c.PathValue
}

func (c *BuiltinEvalContext) SetRootModuleArgument(addr addrs.InputVariable, v cty.Value) {
	c.VariableValuesLock.Lock()
	defer c.VariableValuesLock.Unlock()

	log.Printf("[TRACE] BuiltinEvalContext: Storing final value for variable %s", addr.Absolute(addrs.RootModuleInstance))
	key := addrs.RootModuleInstance.String()
	args := c.VariableValues[key]
	if args == nil {
		args = make(map[string]cty.Value)
		c.VariableValues[key] = args
	}
	args[addr.Name] = v
}

func (c *BuiltinEvalContext) SetModuleCallArgument(callAddr addrs.ModuleCallInstance, varAddr addrs.InputVariable, v cty.Value) {
	c.VariableValuesLock.Lock()
	defer c.VariableValuesLock.Unlock()

	if !c.pathSet {
		panic("context path not set")
	}

	childPath := callAddr.ModuleInstance(c.PathValue)
	log.Printf("[TRACE] BuiltinEvalContext: Storing final value for variable %s", varAddr.Absolute(childPath))
	key := childPath.String()
	args := c.VariableValues[key]
	if args == nil {
		args = make(map[string]cty.Value)
		c.VariableValues[key] = args
	}
	args[varAddr.Name] = v
}

func (c *BuiltinEvalContext) GetVariableValue(addr addrs.AbsInputVariableInstance) cty.Value {
	c.VariableValuesLock.Lock()
	defer c.VariableValuesLock.Unlock()

	modKey := addr.Module.String()
	modVars := c.VariableValues[modKey]
	val, ok := modVars[addr.Variable.Name]
	if !ok {
		return cty.DynamicVal
	}
	return val
}

func (c *BuiltinEvalContext) Changes() *plans.ChangesSync {
	return c.ChangesValue
}

func (c *BuiltinEvalContext) State() *states.SyncState {
	return c.StateValue
}

func (c *BuiltinEvalContext) Checks() *checks.State {
	return c.ChecksValue
}

func (c *BuiltinEvalContext) RefreshState() *states.SyncState {
	return c.RefreshStateValue
}

func (c *BuiltinEvalContext) PrevRunState() *states.SyncState {
	return c.PrevRunStateValue
}

func (c *BuiltinEvalContext) InstanceExpander() *instances.Expander {
	return c.InstanceExpanderValue
}

func (c *BuiltinEvalContext) MoveResults() refactoring.MoveResults {
	return c.MoveResultsValue
}

func (c *BuiltinEvalContext) ImportResolver() *ImportResolver {
	return c.ImportResolverValue
}

func (c *BuiltinEvalContext) GetEncryption() encryption.Encryption {
	return c.Encryption
}
