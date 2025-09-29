// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package execgraph

import (
	"context"
	"fmt"
	"iter"
	"reflect"
	"strings"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

// compilerOperands is a helper for concisely unpacking the operands of an
// operation while asserting the result types they are expected to produce.
//
// Users of this should call [nextOperand] for each expected operand in turn,
// and then call [compilerOperands.Finish] to collect error diagnostics for
// any problems that were detected and to ensure that the internal state is
// cleaned up correctly. If the Finish method returns error diagnostics then
// none of the results from [nextOperand] should be used.
//
//	// assuming that "operands" is a pointer to a compilerOperands object
//	getProviderAddr := nextOperand[addrs.Provider](operands)
//	getProviderConfig := nextOperand[cty.Value](operands)
//	waitForDependencies := operands.OperandWaiter()
//	diags := operands.Finish()
//	if diags.HasErrors() {
//		// compilation fails
//	}
type compilerOperands struct {
	opCode      opCode
	nextOperand func() (AnyResultRef, nodeExecuteRaw, bool)
	stop        func()
	idx         int
	problems    []string
}

// newCompilerOperands prepares a new [compilerOperands] object that produces
// results based on the given sequence of operands, which was presumably
// returned by [compiler.compileOperands].
//
// Refer to the documentation of [compilerOperands] for an example of how to
// use the result.
func newCompilerOperands(opCode opCode, operands iter.Seq2[AnyResultRef, nodeExecuteRaw]) *compilerOperands {
	next, stop := iter.Pull2(operands)
	return &compilerOperands{
		opCode:      opCode,
		nextOperand: next,
		stop:        stop,
		idx:         0,
		problems:    nil,
	}
}

func nextOperand[T any](operands *compilerOperands) nodeExecute[T] {
	idx := operands.idx
	operands.idx++
	resultRef, execRaw, ok := operands.nextOperand()
	if !ok {
		operands.problems = append(operands.problems, fmt.Sprintf("missing expected operand %d", idx))
		return nil
	}
	// We'll catch type mismatches during compile time as long as the compiler
	// produces correct nodeExecuteRaw implementations that actually honor
	// the expected type.
	if _, typeOk := resultRef.(ResultRef[T]); !typeOk {
		var zero T
		ty := reflect.TypeOf(&zero).Elem()
		operands.problems = append(operands.problems, fmt.Sprintf("operand %d not of expected type %s.%s (got %T)", idx, ty.PkgPath(), ty.Name(), resultRef))
		return nil
	}

	return func(ctx context.Context) (T, bool, tfdiags.Diagnostics) {
		var diags tfdiags.Diagnostics
		// We intentionally don't propagate diagnostics here because they
		// describe problems that the node associated with this operand will
		// report directly when visited by [CompiledGraph.Execute]. We only
		// want to return diagnostics that are unique to this particular
		// reference to the node, such as the type mismatch error below.
		resultRaw, ok, _ := execRaw(ctx)
		if !ok {
			var zero T
			return zero, false, nil
		}
		result, ok := resultRaw.(T)
		if !ok {
			// We'll get here if the execRaw function was compiled incorrectly
			// so that its actual result does not agree with the type of the
			// ResultRef it was expected to satisfy.
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid execution graph compilation",
				fmt.Sprintf("Operand %d was supposed to be %T, but its implementation produced %T. This is a bug in OpenTofu.", idx, result, resultRaw),
			))
			var zero T
			return zero, false, diags
		}
		return result, true, diags
	}
}

// OperandWaiter is a variant of [nextOperand] for operands that don't produce
// a useful value and exist only to block beginning some other work until
// they have completed.
//
// If the returned function produces false then the caller must immediately
// return without doing any other work, because some upstream has failed and
// so we need to unwind and report the collected errors.
func (ops *compilerOperands) OperandWaiter() func(ctx context.Context) bool {
	idx := ops.idx
	ops.idx++
	_, execRaw, ok := ops.nextOperand()
	if !ok {
		ops.problems = append(ops.problems, fmt.Sprintf("missing expected operand %d", idx))
		return nil
	}
	return func(ctx context.Context) bool {
		_, canContinue, _ := execRaw(ctx)
		return canContinue
	}
}

func (ops *compilerOperands) Finish() tfdiags.Diagnostics {
	// Regardless of how this terminates we no longer need the operand iterator.
	defer ops.stop()

	var diags tfdiags.Diagnostics
	problems := ops.problems
	if _, _, anotherOperand := ops.nextOperand(); anotherOperand {
		problems = append(problems, fmt.Sprintf("expected only %d operands, but found additional operands", ops.idx))
	}
	if len(problems) != 0 {
		var buf strings.Builder
		fmt.Fprintf(&buf, "Found incorrect operands when compiling %s:\n", ops.opCode)
		for _, problem := range problems {
			fmt.Fprintf(&buf, " - %s\n", problem)
		}
		buf.WriteString("\nThis is a bug in OpenTofu.")
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid operands for execution graph operation",
			buf.String(),
		))
	}
	return diags
}
