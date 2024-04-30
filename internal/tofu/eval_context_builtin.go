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
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/checks"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/lang/marks"
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
	ProviderCache       map[string]providers.Interface
	ProviderInputConfig map[string]map[string]cty.Value

	ProvisionerLock  *sync.Mutex
	ProvisionerCache map[string]provisioners.Interface

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

func (ctx *BuiltinEvalContext) InitProvider(addr addrs.AbsProviderConfig) (providers.Interface, error) {
	ctx.ProviderLock.Lock()
	defer ctx.ProviderLock.Unlock()

	key := addr.String()

	// If we have already initialized, it is an error
	if _, ok := ctx.ProviderCache[key]; ok {
		return nil, fmt.Errorf("%s is already initialized", addr)
	}

	p, err := ctx.Plugins.NewProviderInstance(addr.Provider)
	if err != nil {
		return nil, err
	}

	log.Printf("[TRACE] BuiltinEvalContext: Initialized %q provider for %s", addr.String(), addr)
	ctx.ProviderCache[key] = p

	return p, nil
}

func (ctx *BuiltinEvalContext) Provider(addr addrs.AbsProviderConfig) providers.Interface {
	ctx.ProviderLock.Lock()
	defer ctx.ProviderLock.Unlock()

	return ctx.ProviderCache[addr.String()]
}

func (ctx *BuiltinEvalContext) ProviderSchema(addr addrs.AbsProviderConfig) (providers.ProviderSchema, error) {
	return ctx.Plugins.ProviderSchema(addr.Provider)
}

func (ctx *BuiltinEvalContext) CloseProvider(addr addrs.AbsProviderConfig) error {
	ctx.ProviderLock.Lock()
	defer ctx.ProviderLock.Unlock()

	key := addr.String()
	provider := ctx.ProviderCache[key]
	if provider != nil {
		delete(ctx.ProviderCache, key)
		return provider.Close()
	}

	return nil
}

func (ctx *BuiltinEvalContext) ConfigureProvider(addr addrs.AbsProviderConfig, cfg cty.Value) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if !addr.Module.Equal(ctx.Path().Module()) {
		// This indicates incorrect use of ConfigureProvider: it should be used
		// only from the module that the provider configuration belongs to.
		panic(fmt.Sprintf("%s configured by wrong module %s", addr, ctx.Path()))
	}

	p := ctx.Provider(addr)
	if p == nil {
		diags = diags.Append(fmt.Errorf("%s not initialized", addr))
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

	if !pc.Module.Equal(ctx.Path().Module()) {
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
	cfg := ctx.Evaluator.Config.Descendent(ctx.Path().Module())
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

// EvaluateImportAddress takes the raw reference expression of the import address
// from the config, and returns the evaluated address addrs.AbsResourceInstance
//
// The implementation is inspired by config.AbsTraversalForImportToExpr, but this time we can evaluate the expression
// in the indexes of expressions. If we encounter a hclsyntax.IndexExpr, we can evaluate the Key expression and create
// an Index Traversal, adding it to the Traverser
// TODO move this function into eval_import.go
func (ctx *BuiltinEvalContext) EvaluateImportAddress(expr hcl.Expression, keyData instances.RepetitionData) (addrs.AbsResourceInstance, tfdiags.Diagnostics) {
	traversal, diags := ctx.traversalForImportExpr(expr, keyData)
	if diags.HasErrors() {
		return addrs.AbsResourceInstance{}, diags
	}

	return addrs.ParseAbsResourceInstance(traversal)
}

func (ctx *BuiltinEvalContext) traversalForImportExpr(expr hcl.Expression, keyData instances.RepetitionData) (traversal hcl.Traversal, diags tfdiags.Diagnostics) {
	switch e := expr.(type) {
	case *hclsyntax.IndexExpr:
		t, d := ctx.traversalForImportExpr(e.Collection, keyData)
		diags = diags.Append(d)
		traversal = append(traversal, t...)

		tIndex, dIndex := ctx.parseImportIndexKeyExpr(e.Key, keyData)
		diags = diags.Append(dIndex)
		traversal = append(traversal, tIndex)
	case *hclsyntax.RelativeTraversalExpr:
		t, d := ctx.traversalForImportExpr(e.Source, keyData)
		diags = diags.Append(d)
		traversal = append(traversal, t...)
		traversal = append(traversal, e.Traversal...)
	case *hclsyntax.ScopeTraversalExpr:
		traversal = append(traversal, e.Traversal...)
	default:
		// This should not happen, as it should have failed validation earlier, in config.AbsTraversalForImportToExpr
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid import address expression",
			Detail:   "Import address must be a reference to a resource's address, and only allows for indexing with dynamic keys. For example: module.my_module[expression1].aws_s3_bucket.my_buckets[expression2] for resources inside of modules, or simply aws_s3_bucket.my_bucket for a resource in the root module",
			Subject:  expr.Range().Ptr(),
		})
	}
	return
}

// parseImportIndexKeyExpr parses an expression that is used as a key in an index, of an HCL expression representing an
// import target address, into a traversal of type hcl.TraverseIndex.
// After evaluation, the expression must be known, not null, not sensitive, and must be a string (for_each) or a number
// (count)
func (ctx *BuiltinEvalContext) parseImportIndexKeyExpr(expr hcl.Expression, keyData instances.RepetitionData) (hcl.TraverseIndex, tfdiags.Diagnostics) {
	idx := hcl.TraverseIndex{
		SrcRange: expr.Range(),
	}

	// evaluate and take into consideration the for_each key (if exists)
	val, diags := evaluateExprWithRepetitionData(ctx, expr, cty.DynamicPseudoType, keyData)
	if diags.HasErrors() {
		return idx, diags
	}

	if !val.IsKnown() {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Import block 'to' address contains an invalid key",
			Detail:   "Import block contained a resource address using an index that will only be known after apply. Please ensure to use expressions that are known at plan time for the index of an import target address",
			Subject:  expr.Range().Ptr(),
		})
		return idx, diags
	}

	if val.IsNull() {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Import block 'to' address contains an invalid key",
			Detail:   "Import block contained a resource address using an index which is null. Please ensure the expression for the index is not null",
			Subject:  expr.Range().Ptr(),
		})
		return idx, diags
	}

	if val.Type() != cty.String && val.Type() != cty.Number {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Import block 'to' address contains an invalid key",
			Detail:   "Import block contained a resource address using an index which is not valid for a resource instance (not a string or a number). Please ensure the expression for the index is correct, and returns either a string or a number",
			Subject:  expr.Range().Ptr(),
		})
		return idx, diags
	}

	unmarkedVal, valMarks := val.Unmark()
	if _, sensitive := valMarks[marks.Sensitive]; sensitive {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Import block 'to' address contains an invalid key",
			Detail:   "Import block contained a resource address using an index which is sensitive. Please ensure indexes used in the resource address of an import target are not sensitive",
			Subject:  expr.Range().Ptr(),
		})
	}

	idx.Key = unmarkedVal
	return idx, diags
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
		return evalContextProviderFunction(ctx, mc, ctx.Evaluator.Operation, pf, rng)
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
