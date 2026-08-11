// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Apply represents the command-line arguments for the apply command.
type Apply struct {
	// State, Operation, and Vars are the common extended flags
	State     *State
	Operation *Operation
	Vars      *Vars

	// AutoApprove skips the manual verification step for the apply operation.
	AutoApprove bool

	// PlanPath contains an optional path to a stored plan file
	PlanPath string

	// View represents the global view options
	View *View

	// SuppressForgetErrorsDuringDestroy suppresses the error that occurs when a
	// destroy operation completes successfully but leaves forgotten instances behind.
	SuppressForgetErrorsDuringDestroy bool
}

// BindApply registers CLI arguments, returning a Apply value and it's corresponding hooks.
func BindApply(cli *CommandLine) *Apply {
	apply := Apply{
		View:      BindView(cli, viewFlagAll),
		Operation: BindOperation(cli),
		Vars:      BindVars(cli),
		State:     BindState(cli, stateFlagAll),
	}

	cli.BoolVar(&apply.AutoApprove, "auto-approve", false, "Skip interactive approval of plan before applying.")
	cli.BoolVar(&apply.SuppressForgetErrorsDuringDestroy, "suppress-forget-errors", false, "Suppress the error that occurs when a destroy operation completes successfully but leaves forgotten instances behind.")

	cli.PositionalArg(&apply.PlanPath, "PLAN", true)

	cli.PreHook(func() tfdiags.Diagnostics {
		// JSON view cannot confirm apply, so we require either a plan file or
		// auto-approve to be specified. We intentionally fail here rather than
		// override auto-approve, which would be dangerous.
		if apply.View.ViewType == ViewJSON && apply.PlanPath == "" && !apply.AutoApprove {
			return tfdiags.New(tfdiags.Sourceless(
				tfdiags.Error,
				"Plan file or auto-approve required",
				"OpenTofu cannot ask for interactive approval when -json is set. You can either apply a saved plan file, or enable the -auto-approve option.",
			))
		}
		return nil
	})

	return &apply
}

// BindApplyDestroy registers CLI arguments, returning a Apply value and it's corresponding hooks.
func BindApplyDestroy(cli *CommandLine) *Apply {
	apply := BindApply(cli)

	cli.PreHook(func() tfdiags.Diagnostics {
		// So far ParseApply was using the command line options like -destroy
		// and -refresh-only to determine the plan mode. For "tofu destroy"
		// we expect neither of those arguments to be set, and so the plan mode
		// should currently be set to NormalMode, which we'll replace with
		// DestroyMode here. If it's already set to something else then that
		// suggests incorrect usage.
		switch apply.Operation.PlanMode {
		case plans.NormalMode:
			// This indicates that the user didn't specify any mode options at
			// all, which is correct, although we know from the command that
			// they actually intended to use DestroyMode here.
			apply.Operation.PlanMode = plans.DestroyMode
		case plans.DestroyMode:
			return tfdiags.New(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid mode option",
				"The -destroy option is not valid for \"tofu destroy\", because this command always runs in destroy mode.",
			))
		case plans.RefreshOnlyMode:
			return tfdiags.New(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid mode option",
				"The -refresh-only option is not valid for \"tofu destroy\".",
			))
		default:
			// This is a non-ideal error message for if we forget to handle a
			// newly-handled plan mode in Operation.Parse. Ideally they should all
			// have cases above so we can produce better error messages.
			return tfdiags.New(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid mode option",
				fmt.Sprintf("The \"tofu destroy\" command doesn't support %s.", apply.Operation.PlanMode),
			))
		}

		// NOTE: It's also invalid to have apply.PlanPath set in this codepath,
		// but we don't check that in here because we'll return a different error
		// message depending on whether the given path seems to refer to a saved
		// plan file or to a configuration directory. The apply command
		// implementation itself therefore handles this situation.
		return nil
	})

	return apply
}

// ParseApply processes CLI arguments, returning an Apply value, a closer function, and errors.
// If errors are encountered, an Apply value is still returned representing
// the best effort interpretation of the arguments.
func ParseApply(args []string) (*Apply, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	apply := BindApply(cli)
	closer, diags := cli.parseWithHooks("apply", args)
	return apply, closer, diags
}

// ParseApplyDestroy is a special case of ParseApply that deals with the
// "tofu destroy" command, which is effectively an alias for
// "tofu apply -destroy".
func ParseApplyDestroy(args []string) (*Apply, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	apply := BindApplyDestroy(cli)
	closer, diags := cli.parseWithHooks("destroy", args)
	return apply, closer, diags
}
