// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"log"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// nodeOutputValue represents an "output" block in the configuration, before
// its individual instances have been expanded.
type nodeOutputValue struct {
	Addr        addrs.OutputValue
	Module      addrs.Module
	Config      *configs.Output
	Destroying  bool
	RefreshOnly bool

	// Planning is set to true when this node is in a graph that was produced
	// by the plan graph builder, as opposed to the apply graph builder.
	// This quirk is just because we share the same node type between both
	// phases but in practice there are a few small differences in the actions
	// we need to take between plan and apply. See method Execute for
	// details.
	Planning bool
}

var (
	_ GraphNodeReferenceable    = (*nodeOutputValue)(nil)
	_ GraphNodeReferencer       = (*nodeOutputValue)(nil)
	_ GraphNodeReferenceOutside = (*nodeOutputValue)(nil)
	_ GraphNodeExecutable       = (*nodeOutputValue)(nil)
	_ graphNodeTemporaryValue   = (*nodeOutputValue)(nil)
	_ graphNodeExpandsInstances = (*nodeOutputValue)(nil)
)

func (n *nodeOutputValue) expandsInstances() {}

func (n *nodeOutputValue) temporaryValue() bool {
	// non root outputs are temporary
	return !n.Module.IsRoot()
}

func (n *nodeOutputValue) Execute(evalCtx EvalContext, op walkOperation) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	expander := evalCtx.InstanceExpander()
	changes := evalCtx.Changes()
	moduleInstAddrs := expander.ExpandModule(n.Module)

	if changes == nil {
		panic(fmt.Sprintf("no Changes object for nodeOutputValue %s in %s", n.Name(), op))
	}

	// If this is an output value that participates in custom condition checks
	// (i.e. it has preconditions or postconditions) then the check state
	// wants to know the addresses of the checkable objects so that it can
	// treat them as unknown status if we encounter an error before actually
	// visiting the checks.
	//
	// We must do this only during planning, because the apply phase will start
	// with all of the same checkable objects that were registered during the
	// planning phase. Consumers of our JSON plan and state formats expect
	// that the set of checkable objects will be consistent between the plan
	// and any state snapshots created during apply, and that only the statuses
	// of those objects will have changed.
	if n.Planning {
		if checkState := evalCtx.Checks(); checkState.ConfigHasChecks(n.Addr.InModule(n.Module)) {
			checkableAddrs := addrs.MakeSet[addrs.Checkable]()
			for _, module := range moduleInstAddrs {
				absAddr := n.Addr.Absolute(module)
				checkableAddrs.Add(absAddr)
			}
			checkState.ReportCheckableObjects(n.Addr.InModule(n.Module), checkableAddrs)
		}
	}

	// Output value evaluation does not involve any I/O, and so for simplicity's sake
	// we deal with all of the instances of a output value as sequential code rather
	// than trying to evaluate them concurrently.

	for _, module := range moduleInstAddrs {
		absAddr := n.Addr.Absolute(module)

		// Find any recorded change for this output
		var change *plans.OutputChangeSrc
		var outputChanges []*plans.OutputChangeSrc
		if module.IsRoot() {
			outputChanges = changes.GetRootOutputChanges()
		} else {
			parent, call := module.Call()
			outputChanges = changes.GetOutputChanges(parent, call)
		}
		for _, c := range outputChanges {
			if c.Addr.String() == absAddr.String() {
				change = c
				break
			}
		}

		var execute func(evalCtx EvalContext, addr addrs.AbsOutputValue, change *plans.OutputChangeSrc) tfdiags.Diagnostics
		switch {
		case module.IsRoot() && n.Destroying:
			// Root module output values get totally removed when applying
			// a plan created in the "destroy" planning mode, and this
			// function provides that special treatment. This is also used
			// by [OrphanOutputTransformer] to force destroy treatment for
			// any root module output values that are no longer declared
			// in the configuration.
			execute = n.executeInstanceRootDestroy
		default:
			// In all other cases we evaluate the output value expression
			// and save its result for downstream use.
			execute = n.executeInstance
		}

		log.Printf("[TRACE] nodeOutputValue: evaluating %s", absAddr)
		moduleEvalCtx := evalCtx.WithPath(absAddr.Module)
		diags = diags.Append(execute(moduleEvalCtx, absAddr, change))
	}

	return diags
}

//nolint:cyclop,funlen // This function predates our use of these lints
func (n *nodeOutputValue) executeInstance(evalCtx EvalContext, absAddr addrs.AbsOutputValue, change *plans.OutputChangeSrc) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	state := evalCtx.State()
	if state == nil {
		return diags
	}

	changes := evalCtx.Changes() // may be nil, if we're not working on a changeset

	val := cty.UnknownVal(cty.DynamicPseudoType)
	changeRecorded := change != nil
	// we we have a change recorded, we don't need to re-evaluate if the value
	// was known
	if changeRecorded {
		change, err := change.Decode()
		diags = diags.Append(err)
		if err == nil {
			val = change.After
		}
	}

	// Checks are not evaluated during a destroy. The checks may fail, may not
	// be valid, or may not have been registered at all.
	if !n.Destroying {
		checkRuleSeverity := tfdiags.Error
		if n.RefreshOnly {
			checkRuleSeverity = tfdiags.Warning
		}
		checkDiags := evalCheckRules(
			addrs.OutputPrecondition,
			n.Config.Preconditions,
			evalCtx, absAddr, EvalDataForNoInstanceKey,
			checkRuleSeverity,
		)
		diags = diags.Append(checkDiags)
		if diags.HasErrors() {
			return diags // failed preconditions prevent further evaluation
		}
	}

	// If there was no change recorded, or the recorded change was not wholly
	// known, then we need to re-evaluate the output
	if !changeRecorded || !val.IsWhollyKnown() {
		switch {
		// If the module is not being overridden, we proceed normally
		case !n.Config.IsOverridden:
			// This has to run before we have a state lock, since evaluation also
			// reads the state
			var evalDiags tfdiags.Diagnostics
			val, evalDiags = evalCtx.EvaluateExpr(n.Config.Expr, cty.DynamicPseudoType, nil)
			diags = diags.Append(evalDiags)

		// If the module is being overridden and we have a value to use,
		// we just use it
		case n.Config.OverrideValue != nil:
			val = *n.Config.OverrideValue

		// If the module is being overridden, but we don't have any value to use,
		// we just set it to null
		default:
			val = cty.NilVal
		}

		// We'll handle errors below, after we have loaded the module.
		// Outputs don't have a separate mode for validation, so validate
		// depends_on expressions here too
		diags = diags.Append(validateDependsOn(evalCtx, n.Config.DependsOn))

		// For root module outputs in particular, an output value must be
		// statically declared as sensitive in order to dynamically return
		// a sensitive result, to help avoid accidental exposure in the state
		// of a sensitive value that the user doesn't want to include there.
		if absAddr.Module.IsRoot() {
			if !n.Config.Sensitive && marks.Contains(val, marks.Sensitive) {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Output refers to sensitive values",
					Detail: `To reduce the risk of accidentally exporting sensitive data that was intended to be only internal, OpenTofu requires that any root module output containing sensitive data be explicitly marked as sensitive, to confirm your intent.

If you do intend to export this data, annotate the output value as sensitive by adding the following argument:
    sensitive = true`,
					Subject: n.Config.DeclRange.Ptr(),
				})
			}
		}
	}

	// handling the interpolation error
	if diags.HasErrors() {
		if flagWarnOutputErrors {
			log.Printf("[ERROR] Output interpolation %q failed: %s", n.Addr, diags.Err())
			// if we're continuing, make sure the output is included, and
			// marked as unknown. If the evaluator was able to find a type
			// for the value in spite of the error then we'll use it.
			n.setValue(absAddr, state, changes, cty.UnknownVal(val.Type()))

			// Keep existing warnings, while converting errors to warnings.
			// This is not meant to be the normal path, so there no need to
			// make the errors pretty.
			var warnings tfdiags.Diagnostics
			for _, d := range diags {
				switch d.Severity() {
				case tfdiags.Warning:
					warnings = warnings.Append(d)
				case tfdiags.Error:
					desc := d.Description()
					warnings = warnings.Append(tfdiags.SimpleWarning(fmt.Sprintf("%s:%s", desc.Summary, desc.Detail)))
				}
			}

			return warnings
		}
		return diags
	}
	n.setValue(absAddr, state, changes, val)

	// If we were able to evaluate a new value, we can update that in the
	// refreshed state as well.
	if state = evalCtx.RefreshState(); state != nil && val.IsWhollyKnown() {
		// we only need to update the state, do not pass in the changes again
		n.setValue(absAddr, state, nil, val)
	}

	return diags
}

// executeInstanceRootDestroy is a variant of [nodeExpandOutput.executeInstance] that deals
// only with root module outputs when the planning mode is "destroy" and thus our goal is
// to end up with an empty state that has no output values at all.
func (n *nodeOutputValue) executeInstanceRootDestroy(evalCtx EvalContext, absAddr addrs.AbsOutputValue, _ *plans.OutputChangeSrc) tfdiags.Diagnostics {
	state := evalCtx.State()
	if state == nil {
		return nil
	}

	// if this is a root module, try to get a before value from the state for
	// the diff
	sensitiveBefore := false
	before := cty.NullVal(cty.DynamicPseudoType)
	mod := state.Module(absAddr.Module)
	if absAddr.Module.IsRoot() && mod != nil {
		if o, ok := mod.OutputValues[absAddr.OutputValue.Name]; ok {
			sensitiveBefore = o.Sensitive
			before = o.Value
		} else {
			// If the output was not in state, a delete change would
			// be meaningless, so exit early.
			return nil

		}
	}

	changes := evalCtx.Changes()
	if changes != nil && n.Planning {
		change := &plans.OutputChange{
			Addr:      absAddr,
			Sensitive: sensitiveBefore,
			Change: plans.Change{
				Action: plans.Delete,
				Before: before,
				After:  cty.NullVal(cty.DynamicPseudoType),
			},
		}

		cs, err := change.Encode()
		if err != nil {
			// Should never happen, since we just constructed this right above
			panic(fmt.Sprintf("planned change for %s could not be encoded: %s", n.Addr, err))
		}
		log.Printf("[TRACE] nodeOutputValue: Saving %s change for %s in changeset", change.Action, n.Addr)

		changes.RemoveOutputChange(absAddr) // remove any existing planned change, if present
		changes.AppendOutputChange(cs)      // add the new planned change
	}

	state.RemoveOutputValue(absAddr)
	return nil

}

func (n *nodeOutputValue) setValue(absAddr addrs.AbsOutputValue, state *states.SyncState, changes *plans.ChangesSync, val cty.Value) {
	if changes != nil && n.Planning {
		// if this is a root module, try to get a before value from the state for
		// the diff
		sensitiveBefore := false
		before := cty.NullVal(cty.DynamicPseudoType)

		// is this output new to our state?
		newOutput := true

		mod := state.Module(absAddr.Module)
		if absAddr.Module.IsRoot() && mod != nil {
			for name, o := range mod.OutputValues {
				if name == absAddr.OutputValue.Name {
					before = o.Value
					sensitiveBefore = o.Sensitive
					newOutput = false
					break
				}
			}
		}

		// We will not show the value if either the before or after are marked
		// as sensitive. We can show the value again once sensitivity is
		// removed from both the config and the state.
		sensitiveChange := sensitiveBefore || n.Config.Sensitive

		// strip any marks here just to be sure we don't panic on the True comparison
		unmarkedVal, _ := val.UnmarkDeep()

		action := plans.Update
		switch {
		case val.IsNull() && before.IsNull():
			// This is separate from the NoOp case below, since we can ignore
			// sensitivity here when there are only null values.
			action = plans.NoOp

		case newOutput:
			// This output was just added to the configuration
			action = plans.Create

		case val.IsWhollyKnown() &&
			unmarkedVal.Equals(before).True() &&
			n.Config.Sensitive == sensitiveBefore:
			// Sensitivity must also match to be a NoOp.
			// Theoretically marks may not match here, but sensitivity is the
			// only one we can act on, and the state will have been loaded
			// without any marks to consider.
			action = plans.NoOp
		}

		change := &plans.OutputChange{
			Addr:      absAddr,
			Sensitive: sensitiveChange,
			Change: plans.Change{
				Action: action,
				Before: before,
				After:  val,
			},
		}

		cs, err := change.Encode()
		if err != nil {
			// Should never happen, since we just constructed this right above
			panic(fmt.Sprintf("planned change for %s could not be encoded: %s", n.Addr, err))
		}
		log.Printf("[TRACE] nodeOutputValue: Saving %s change for %s in changeset", change.Action, n.Addr)
		changes.AppendOutputChange(cs) // add the new planned change
	}

	if changes != nil && !n.Planning {
		// During apply there is no longer any change to track, so we must
		// ensure the state is updated and not overridden by a change.
		changes.RemoveOutputChange(absAddr)
	}

	// Null outputs must be saved for modules so that they can still be
	// evaluated. Null root outputs are removed entirely, which is always fine
	// because they can't be referenced by anything else in the configuration.
	if absAddr.Module.IsRoot() && val.IsNull() {
		log.Printf("[TRACE] setValue: Removing %s from state (it is now null)", n.Addr)
		state.RemoveOutputValue(absAddr)
		return
	}

	log.Printf("[TRACE] setValue: Saving value for %s in state", n.Addr)

	// non-root outputs need to keep sensitive marks for evaluation, but are
	// not serialized.
	if absAddr.Module.IsRoot() {
		val, _ = val.UnmarkDeep()
		val = cty.UnknownAsNull(val)
	}

	state.SetOutputValue(absAddr, val, n.Config.Sensitive)
}

func (n *nodeOutputValue) Name() string {
	path := n.Module.String()
	addr := n.Addr.String()
	if path != "" {
		return path + "." + addr
	}
	return addr
}

// GraphNodeModulePath
func (n *nodeOutputValue) ModulePath() addrs.Module {
	return n.Module
}

// GraphNodeReferenceable
func (n *nodeOutputValue) ReferenceableAddrs() []addrs.Referenceable {
	// An output in the root module can't be referenced at all.
	if n.Module.IsRoot() {
		return nil
	}

	// the output is referenced through the module call, and via the
	// module itself.
	_, call := n.Module.Call()
	callOutput := addrs.ModuleCallOutput{
		Call: call,
		Name: n.Addr.Name,
	}

	// Otherwise, we can reference the output via the
	// module call itself
	return []addrs.Referenceable{call, callOutput}
}

// GraphNodeReferenceOutside implementation
func (n *nodeOutputValue) ReferenceOutside() (addrs.Module, addrs.Module) {
	// Output values have their expressions resolved in the context of the
	// module where they are defined.
	referencePath := n.Module

	// ...but they are referenced in the context of their calling module.
	selfPath := referencePath.Parent()

	return selfPath, referencePath
}

// GraphNodeReferencer
func (n *nodeOutputValue) References() []*addrs.Reference {
	// DestroyNodes do not reference anything.
	if n.Module.IsRoot() && n.Destroying {
		return nil
	}

	return referencesForOutput(n.Config)
}

func referencesForOutput(c *configs.Output) []*addrs.Reference {
	var refs []*addrs.Reference

	impRefs, _ := lang.ReferencesInExpr(addrs.ParseRef, c.Expr)
	expRefs, _ := lang.References(addrs.ParseRef, c.DependsOn)

	refs = append(refs, impRefs...)
	refs = append(refs, expRefs...)

	for _, check := range c.Preconditions {
		condRefs, _ := lang.ReferencesInExpr(addrs.ParseRef, check.Condition)
		refs = append(refs, condRefs...)
		errRefs, _ := lang.ReferencesInExpr(addrs.ParseRef, check.ErrorMessage)
		refs = append(refs, errRefs...)
	}

	return refs
}
