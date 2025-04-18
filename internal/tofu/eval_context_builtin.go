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
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/provisioners"
	"github.com/opentofu/opentofu/internal/refactoring"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/version"
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
	Plugins *contextPlugins

	Hooks      []Hook
	InputValue UIInput

	ProviderLock        *sync.Mutex
	ProviderCache       map[string]map[addrs.InstanceKey]providers.Interface
	ProviderInputConfig map[string]map[string]cty.Value

	ProvisionerLock  *sync.Mutex
	ProvisionerCache map[string]provisioners.Interface

	ChangesValue            *plans.ChangesSync
	StateValue              *states.SyncState
	ChecksValue             *checks.State
	RefreshStateValue       *states.SyncState
	PrevRunStateValue       *states.SyncState
	InstanceExpanderValue   *instances.Expander
	MoveResultsValue        refactoring.MoveResults
	ImportResolverValue     *ImportResolver
	Encryption              encryption.Encryption
	ProviderFunctionTracker ProviderFunctionMapping
}

// BuiltinEvalContext implements EvalContext
var _ EvalContext = (*BuiltinEvalContext)(nil)

func (ctx *BuiltinEvalContext) WithPath(path addrs.ModuleInstance) EvalContext {
	newCtx := *ctx
	newCtx.pathSet = true
	newCtx.PathValue = path
	return &newCtx
}

func (ctx *BuiltinEvalContext) Stopped() <-chan struct{} {
	// This can happen during tests. During tests, we just block forever.
	if ctx.StopContext == nil {
		return nil
	}

	return ctx.StopContext.Done()
}

func (ctx *BuiltinEvalContext) Hook(fn func(Hook) (HookAction, error)) error {
	for _, h := range ctx.Hooks {
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

func (ctx *BuiltinEvalContext) Input() UIInput {
	return ctx.InputValue
}

func (ctx *BuiltinEvalContext) InitProvider(addr addrs.AbsProviderConfig, providerKey addrs.InstanceKey) (providers.Interface, error) {
	ctx.ProviderLock.Lock()
	defer ctx.ProviderLock.Unlock()

	key := addr.String()

	if ctx.ProviderCache[key] == nil {
		ctx.ProviderCache[key] = make(map[addrs.InstanceKey]providers.Interface)
	}

	// If we have already initialized, it is an error
	if _, ok := ctx.ProviderCache[key][providerKey]; ok {
		return nil, fmt.Errorf("%s is already initialized", addr)
	}

	p, err := ctx.Plugins.NewProviderInstance(addr.Provider)
	if err != nil {
		return nil, err
	}

	if ctx.Evaluator != nil && ctx.Evaluator.Config != nil && ctx.Evaluator.Config.Module != nil {
		// If an aliased provider is mocked, we use providerForTest wrapper.
		// We cannot wrap providers.Factory itself, because factories don't support aliases.
		pc, ok := ctx.Evaluator.Config.Module.GetProviderConfig(addr.Provider.Type, addr.Alias)
		if ok && pc.IsMocked {
			testP, err := newProviderForTestWithSchema(p, p.GetProviderSchema())
			if err != nil {
				return nil, err
			}

			p = testP.
				withMockResources(pc.MockResources).
				withOverrideResources(pc.OverrideResources)
		}
	}

	log.Printf("[TRACE] BuiltinEvalContext: Initialized %q%s provider for %s", addr.String(), providerKey, addr)
	ctx.ProviderCache[key][providerKey] = p

	return p, nil
}

func (ctx *BuiltinEvalContext) Provider(addr addrs.AbsProviderConfig, key addrs.InstanceKey) providers.Interface {
	ctx.ProviderLock.Lock()
	defer ctx.ProviderLock.Unlock()

	pm, ok := ctx.ProviderCache[addr.String()]
	if !ok {
		return nil
	}

	return pm[key]
}

func (ctx *BuiltinEvalContext) ProviderSchema(addr addrs.AbsProviderConfig) (providers.ProviderSchema, error) {
	return ctx.Plugins.ProviderSchema(addr.Provider)
}

func (ctx *BuiltinEvalContext) CloseProvider(addr addrs.AbsProviderConfig) error {
	ctx.ProviderLock.Lock()
	defer ctx.ProviderLock.Unlock()

	var diags tfdiags.Diagnostics

	key := addr.String()
	providerMap := ctx.ProviderCache[key]
	if providerMap != nil {
		for _, provider := range providerMap {
			err := provider.Close()
			if err != nil {
				diags = diags.Append(err)
			}
		}
		delete(ctx.ProviderCache, key)
	}
	if diags.HasErrors() {
		return diags.Err()
	}

	return nil
}

func (ctx *BuiltinEvalContext) ConfigureProvider(addr addrs.AbsProviderConfig, providerKey addrs.InstanceKey, cfg cty.Value) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if !ctx.Path().IsForModule(addr.Module) {
		// This indicates incorrect use of ConfigureProvider: it should be used
		// only from the module that the provider configuration belongs to.
		panic(fmt.Sprintf("%s configured by wrong module %s", addr, ctx.Path()))
	}

	p := ctx.Provider(addr, providerKey)
	if p == nil {
		diags = diags.Append(fmt.Errorf("%s not initialized", addr.InstanceString(providerKey)))
		return diags
	}

	req := providers.ConfigureProviderRequest{
		TerraformVersion: version.String(),
		Config:           cfg,
	}

	resp := p.ConfigureProvider(req)
	return resp.Diagnostics
}

func (ctx *BuiltinEvalContext) ProviderInput(pc addrs.AbsProviderConfig) map[string]cty.Value {
	ctx.ProviderLock.Lock()
	defer ctx.ProviderLock.Unlock()

	if !ctx.Path().IsForModule(pc.Module) {
		// This indicates incorrect use of InitProvider: it should be used
		// only from the module that the provider configuration belongs to.
		panic(fmt.Sprintf("%s initialized by wrong module %s", pc, ctx.Path()))
	}

	if !ctx.Path().IsRoot() {
		// Only root module provider configurations can have input.
		return nil
	}

	return ctx.ProviderInputConfig[pc.String()]
}

func (ctx *BuiltinEvalContext) SetProviderInput(pc addrs.AbsProviderConfig, c map[string]cty.Value) {
	absProvider := pc
	if !pc.Module.IsRoot() {
		// Only root module provider configurations can have input.
		log.Printf("[WARN] BuiltinEvalContext: attempt to SetProviderInput for non-root module")
		return
	}

	// Save the configuration
	ctx.ProviderLock.Lock()
	ctx.ProviderInputConfig[absProvider.String()] = c
	ctx.ProviderLock.Unlock()
}

func (ctx *BuiltinEvalContext) Provisioner(n string) (provisioners.Interface, error) {
	ctx.ProvisionerLock.Lock()
	defer ctx.ProvisionerLock.Unlock()

	p, ok := ctx.ProvisionerCache[n]
	if !ok {
		var err error
		p, err = ctx.Plugins.NewProvisionerInstance(n)
		if err != nil {
			return nil, err
		}

		ctx.ProvisionerCache[n] = p
	}

	return p, nil
}

func (ctx *BuiltinEvalContext) ProvisionerSchema(n string) (*configschema.Block, error) {
	return ctx.Plugins.ProvisionerSchema(n)
}

func (ctx *BuiltinEvalContext) CloseProvisioners() error {
	var diags tfdiags.Diagnostics
	ctx.ProvisionerLock.Lock()
	defer ctx.ProvisionerLock.Unlock()

	for name, prov := range ctx.ProvisionerCache {
		err := prov.Close()
		if err != nil {
			diags = diags.Append(fmt.Errorf("provisioner.Close %s: %w", name, err))
		}
	}

	return diags.Err()
}

func (ctx *BuiltinEvalContext) EvaluateBlock(body hcl.Body, schema *configschema.Block, self addrs.Referenceable, keyData InstanceKeyEvalData) (cty.Value, hcl.Body, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	scope := ctx.EvaluationScope(self, nil, keyData)
	body, evalDiags := scope.ExpandBlock(body, schema)
	diags = diags.Append(evalDiags)
	val, evalDiags := scope.EvalBlock(body, schema)
	diags = diags.Append(evalDiags)
	return val, body, diags
}

func (ctx *BuiltinEvalContext) EvaluateExpr(expr hcl.Expression, wantType cty.Type, self addrs.Referenceable) (cty.Value, tfdiags.Diagnostics) {
	scope := ctx.EvaluationScope(self, nil, EvalDataForNoInstanceKey)
	return scope.EvalExpr(expr, wantType)
}

func (ctx *BuiltinEvalContext) EvaluateReplaceTriggeredBy(expr hcl.Expression, repData instances.RepetitionData) (*addrs.Reference, bool, tfdiags.Diagnostics) {

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
		rc := sub.Absolute(ctx.Path())
		changes = ctx.Changes().GetChangesForAbsResource(rc)
	case addrs.ResourceInstance:
		resourceAddr = sub.ContainingResource()
		rc := sub.Absolute(ctx.Path())
		change := ctx.Changes().GetResourceInstanceChange(rc, states.CurrentGen)
		if change != nil {
			// we'll generate an error below if there was no change
			changes = append(changes, change)
		}
	}

	// Do some validation to make sure we are expecting a change at all
	cfg := ctx.Evaluator.Config.DescendentForInstance(ctx.Path())
	resCfg := cfg.Module.ResourceByAddr(resourceAddr)
	if resCfg == nil {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  `Reference to undeclared resource`,
			Detail:   fmt.Sprintf(`A resource %s has not been declared in %s`, ref.Subject, moduleDisplayAddr(ctx.Path())),
			Subject:  expr.Range().Ptr(),
		})
		return nil, false, diags
	}

	if len(changes) == 0 {
		// If the resource is valid there should always be at least one change.
		diags = diags.Append(fmt.Errorf("no change found for %s in %s", ref.Subject, moduleDisplayAddr(ctx.Path())))
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
	schema, err := ctx.ProviderSchema(providerAddr)
	if err != nil {
		diags = diags.Append(err)
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

func (ctx *BuiltinEvalContext) EvaluationScope(self addrs.Referenceable, source addrs.Referenceable, keyData InstanceKeyEvalData) *lang.Scope {
	if !ctx.pathSet {
		panic("context path not set")
	}
	data := &evaluationStateData{
		Evaluator:       ctx.Evaluator,
		ModulePath:      ctx.PathValue,
		InstanceKeyData: keyData,
		Operation:       ctx.Evaluator.Operation,
	}

	// ctx.PathValue is the path of the module that contains whatever
	// expression the caller will be trying to evaluate, so this will
	// activate only the experiments from that particular module, to
	// be consistent with how experiment checking in the "configs"
	// package itself works. The nil check here is for robustness in
	// incompletely-mocked testing situations; mc should never be nil in
	// real situations.
	mc := ctx.Evaluator.Config.DescendentForInstance(ctx.PathValue)

	if mc == nil || mc.Module.ProviderRequirements == nil {
		return ctx.Evaluator.Scope(data, self, source, nil)
	}

	scope := ctx.Evaluator.Scope(data, self, source, func(pf addrs.ProviderFunction, rng tfdiags.SourceRange) (*function.Function, tfdiags.Diagnostics) {
		providedBy, ok := ctx.ProviderFunctionTracker.Lookup(ctx.PathValue.Module(), pf)
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
		if providedBy.KeyExpression != nil && ctx.Evaluator.Operation != walkValidate {
			moduleInstanceForKey := ctx.PathValue[:len(providedBy.KeyModule)]
			if !moduleInstanceForKey.IsForModule(providedBy.KeyModule) {
				panic(fmt.Sprintf("Invalid module key expression location %s in function %s", providedBy.KeyModule, pf.String()))
			}

			var keyDiags tfdiags.Diagnostics
			providerKey, keyDiags = resolveProviderModuleInstance(ctx, providedBy.KeyExpression, moduleInstanceForKey, ctx.PathValue.String()+" "+pf.String())
			if keyDiags.HasErrors() {
				return nil, keyDiags
			}
		}

		provider := ctx.Provider(providedBy.Provider, providerKey)

		if provider == nil {
			// This should not be possible if references are tracked correctly
			return nil, tfdiags.Diagnostics{}.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Uninitialized function provider",
				Detail:   fmt.Sprintf("Provider %q has not yet been initialized", providedBy.Provider.String()),
				Subject:  rng.ToHCL().Ptr(),
			})
		}

		return evalContextProviderFunction(provider, ctx.Evaluator.Operation, pf, rng)
	})
	scope.SetActiveExperiments(mc.Module.ActiveExperiments)

	return scope
}

func (ctx *BuiltinEvalContext) Path() addrs.ModuleInstance {
	if !ctx.pathSet {
		panic("context path not set")
	}
	return ctx.PathValue
}

func (ctx *BuiltinEvalContext) SetRootModuleArgument(addr addrs.InputVariable, v cty.Value) {
	ctx.VariableValuesLock.Lock()
	defer ctx.VariableValuesLock.Unlock()

	log.Printf("[TRACE] BuiltinEvalContext: Storing final value for variable %s", addr.Absolute(addrs.RootModuleInstance))
	key := addrs.RootModuleInstance.String()
	args := ctx.VariableValues[key]
	if args == nil {
		args = make(map[string]cty.Value)
		ctx.VariableValues[key] = args
	}
	args[addr.Name] = v
}

func (ctx *BuiltinEvalContext) SetModuleCallArgument(callAddr addrs.ModuleCallInstance, varAddr addrs.InputVariable, v cty.Value) {
	ctx.VariableValuesLock.Lock()
	defer ctx.VariableValuesLock.Unlock()

	if !ctx.pathSet {
		panic("context path not set")
	}

	childPath := callAddr.ModuleInstance(ctx.PathValue)
	log.Printf("[TRACE] BuiltinEvalContext: Storing final value for variable %s", varAddr.Absolute(childPath))
	key := childPath.String()
	args := ctx.VariableValues[key]
	if args == nil {
		args = make(map[string]cty.Value)
		ctx.VariableValues[key] = args
	}
	args[varAddr.Name] = v
}

func (ctx *BuiltinEvalContext) GetVariableValue(addr addrs.AbsInputVariableInstance) cty.Value {
	ctx.VariableValuesLock.Lock()
	defer ctx.VariableValuesLock.Unlock()

	modKey := addr.Module.String()
	modVars := ctx.VariableValues[modKey]
	val, ok := modVars[addr.Variable.Name]
	if !ok {
		return cty.DynamicVal
	}
	return val
}

func (ctx *BuiltinEvalContext) Changes() *plans.ChangesSync {
	return ctx.ChangesValue
}

func (ctx *BuiltinEvalContext) State() *states.SyncState {
	return ctx.StateValue
}

func (ctx *BuiltinEvalContext) Checks() *checks.State {
	return ctx.ChecksValue
}

func (ctx *BuiltinEvalContext) RefreshState() *states.SyncState {
	return ctx.RefreshStateValue
}

func (ctx *BuiltinEvalContext) PrevRunState() *states.SyncState {
	return ctx.PrevRunStateValue
}

func (ctx *BuiltinEvalContext) InstanceExpander() *instances.Expander {
	return ctx.InstanceExpanderValue
}

func (ctx *BuiltinEvalContext) MoveResults() refactoring.MoveResults {
	return ctx.MoveResultsValue
}

func (ctx *BuiltinEvalContext) ImportResolver() *ImportResolver {
	return ctx.ImportResolverValue
}

func (ctx *BuiltinEvalContext) GetEncryption() encryption.Encryption {
	return ctx.Encryption
}
