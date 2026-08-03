// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/command/flags"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// DefaultParallelism is the limit OpenTofu places on total parallel
// operations as it walks the dependency graph.
const DefaultParallelism = 10

type stateFlag uint8

const (
	stateFlagLock stateFlag = 1 << iota
	stateFlagStateIn
	stateFlagStateOut
	stateFlagBackup

	stateFlagAll = stateFlagLock | stateFlagStateIn | stateFlagStateOut | stateFlagBackup
)

// State describes arguments which are used to define how OpenTofu interacts
// with state.
type State struct {
	// Lock controls whether or not the state manager is used to lock state
	// during operations.
	Lock bool

	// LockTimeout allows setting a time limit on acquiring the state lock.
	// The default is 0, meaning no limit.
	LockTimeout time.Duration

	// StatePath specifies a non-default location for the state file. The
	// default value is blank, which is interpreted as "terraform.tfstate".
	// Represents the local path where state is read from.
	StatePath string

	// StateOutPath specifies a different path to write the final state file.
	// The default value is blank, which results in state being written back to
	// StatePath.
	StateOutPath string

	// BackupPath specifies the path where a backup copy of the state file will
	// be stored before the new state is written. The default value is blank,
	// which is interpreted as StateOutPath + ".backup" or, in some cases, the backup
	// is skipped altogether.
	BackupPath string
}

// bind is the sole logic of registering the state related flags in OpenTofu.
func BindState(cli *CommandLine, mask stateFlag) *State {
	var s State
	cli.State = &s
	if mask&stateFlagLock != 0 {
		cli.BoolVar(&s.Lock, "lock", true,
			`Don't hold a state lock during the operation. This is dangerous if others might concurrently run commands against the same workspace.`,
		).SetDisplay("=false")
		cli.DurationVar(&s.LockTimeout, "lock-timeout", 0,
			`Duration to retry a state lock, such as "5s" to represent five seconds.`,
		).SetDisplay("=duration")
	}
	if mask&stateFlagStateIn != 0 {
		cli.StringVar(&s.StatePath, "state", "",
			`A legacy option used for the local backend only. Refer to the local backend's documentation for more information.`,
		).SetDisplay("=statefile").SetHidden(true)
	}
	if mask&stateFlagStateOut != 0 {
		cli.StringVar(&s.StateOutPath, "state-out", "",
			`Path to write state to that is different than "-state". This can be used to preserve the old state.`,
		).SetHidden(true)
	}
	if mask&stateFlagBackup != 0 {
		cli.StringVar(&s.BackupPath, "backup", "",
			`Path to backup the existing state file before modifying. Defaults to the "-state-out" path with ".backup" extension. Set to "-" to disable backup.`,
		).SetHidden(true)
	}
	return &s
}

// Operation describes arguments which are used to configure how a OpenTofu
// operation such as a plan or apply executes.
type Operation struct {
	// PlanMode selects one of the mutually-exclusive planning modes that
	// decides the overall goal of a plan operation. This field is relevant
	// only for an operation that produces a plan.
	PlanMode plans.Mode

	// Parallelism is the limit OpenTofu places on total parallel operations
	// as it walks the dependency graph.
	Parallelism int

	// Refresh controls whether or not the operation should refresh existing
	// state before proceeding. Default is true.
	Refresh bool

	// Targets allow limiting an operation to a set of resource addresses and
	// their dependencies.
	Targets []addrs.Targetable

	// Excludes allow limiting an operation to execute on all resources other
	// than a set of excluded resource addresses and resources dependent on them.
	Excludes []addrs.Targetable

	// ForceReplace addresses cause OpenTofu to force a particular set of
	// resource instances to generate "replace" actions in any plan where they
	// would normally have generated "no-op" or "update" actions.
	//
	// This is currently limited to specific instances because typical uses
	// of replace are associated with only specific remote objects that the
	// user has somehow learned to be malfunctioning, in which case it
	// would be unusual and potentially dangerous to replace everything under
	// a module all at once. We could potentially loosen this later if we
	// learn a use-case for broader matching.
	ForceReplace []addrs.AbsResourceInstance
}

// parseDirectTargetables gets a list of strings passed from directly from the CLI
// with each representing a targetable object, and returns a list of addrs.Targetable
// This is used for parsing the input of -target and -exclude flags
func parseDirectTargetables(rawTargetables []string, flag string) ([]addrs.Targetable, tfdiags.Diagnostics) {
	var targetables []addrs.Targetable
	var diags tfdiags.Diagnostics

	for _, tr := range rawTargetables {
		traversal, syntaxDiags := hclsyntax.ParseTraversalAbs([]byte(tr), "", hcl.Pos{Line: 1, Column: 1})
		if syntaxDiags.HasErrors() {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				fmt.Sprintf("Invalid %s %q", flag, tr),
				syntaxDiags[0].Detail,
			))
			continue
		}

		target, targetDiags := addrs.ParseTarget(traversal)
		if targetDiags.HasErrors() {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				fmt.Sprintf("Invalid %s %q", flag, tr),
				targetDiags[0].Description().Detail,
			))
			continue
		}

		targetables = append(targetables, target.Subject)
	}
	return targetables, diags
}

// parseFile gets a filePath and reads the file, which contains a list of targets
// with each line in the file representating a targeted object, and returns
// a list of addrs.Targetable. This is used for parsing the input of -target-file
// and -exclude-file flags
func parseFileTargetables(filePaths []string, flag string) ([]addrs.Targetable, tfdiags.Diagnostics) {

	// If no file passed, no targets
	if len(filePaths) <= 0 {
		return nil, nil
	}
	var targetables []addrs.Targetable
	var diags tfdiags.Diagnostics

	for _, filePath := range filePaths {
		b, err := os.ReadFile(filePath)
		if err != nil {
			diags = diags.Append(err)
			continue
		}

		sc := hcl.NewRangeScanner(b, filePath, bufio.ScanLines)
		for sc.Scan() {
			lineBytes := sc.Bytes()
			lineRange := sc.Range()
			if isComment(lineBytes) {
				continue
			}
			traversal, syntaxDiags := hclsyntax.ParseTraversalAbs(lineBytes, lineRange.Filename, lineRange.Start)
			diags = diags.Append(syntaxDiags)
			if syntaxDiags.HasErrors() {
				continue
			}
			target, targetDiags := addrs.ParseTarget(traversal)
			diags = diags.Append(targetDiags)
			if targetDiags.HasErrors() {
				continue
			}
			targetables = append(targetables, target.Subject)
		}

	}
	return targetables, diags
}

func isComment(b []byte) bool {
	return bytes.HasPrefix(bytes.TrimSpace(b), []byte("#"))
}

func parseRawTargetsAndExcludes(targetsDirect, excludesDirect []string, targetFiles, excludeFiles []string) ([]addrs.Targetable, []addrs.Targetable, tfdiags.Diagnostics) {
	var allParsedTargets, allParsedExcludes, parsedTargets []addrs.Targetable
	var parseDiags, diags tfdiags.Diagnostics

	// Cannot exclude and target in same command
	if (len(targetsDirect) > 0 || len(targetFiles) > 0) && (len(excludesDirect) > 0 || len(excludeFiles) > 0) {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid combination of arguments",
			"The target and exclude planning options are mutually-exclusive. Each plan must use either only the target options or only the exclude options.",
		))
		return allParsedTargets, allParsedExcludes, diags
	}

	parsedTargets, parseDiags = parseDirectTargetables(targetsDirect, "target")
	diags = diags.Append(parseDiags)
	allParsedTargets = append(allParsedTargets, parsedTargets...)
	parsedTargets, parseDiags = parseFileTargetables(targetFiles, "target")
	diags = diags.Append(parseDiags)
	allParsedTargets = append(allParsedTargets, parsedTargets...)

	parsedTargets, parseDiags = parseDirectTargetables(excludesDirect, "exclude")
	diags = diags.Append(parseDiags)
	allParsedExcludes = append(allParsedExcludes, parsedTargets...)
	parsedTargets, parseDiags = parseFileTargetables(excludeFiles, "exclude")
	diags = diags.Append(parseDiags)
	allParsedExcludes = append(allParsedExcludes, parsedTargets...)

	return allParsedTargets, allParsedExcludes, diags
}

// bind registers all Operation flags
func BindOperation(cli *CommandLine) *Operation {
	var o Operation
	cli.Operation = &o

	// These private fields are used only temporarily during decoding. Use
	// method Parse to populate the exported fields from these, validating
	// the raw values in the process.
	var targetsRaw []string
	var targetsFilesRaw []string
	var excludesRaw []string
	var excludesFilesRaw []string
	var forceReplaceRaw []string
	var destroyRaw bool
	var refreshOnlyRaw bool

	cli.IntVar(&o.Parallelism, "parallelism", DefaultParallelism,
		`Limit the number of parallel resource operations. Defaults to 10.`,
	).SetDisplay("=n")
	cli.BoolVar(&o.Refresh, "refresh", true,
		`Skip checking for external changes to remote objects while creating the plan. This can potentially make planning faster, but at the expense of possibly planning against a stale record of the remote system state.`,
	).SetDisplay("=false")
	cli.BoolVar(&destroyRaw, "destroy", false,
		`Select the "destroy" planning mode, which creates a plan to destroy all objects currently managed by this OpenTofu configuration instead of the usual behavior.`)
	cli.BoolVar(&refreshOnlyRaw, "refresh-only", false,
		`Select the "refresh only" planning mode, which checks whether remote objects still match the outcome of the most recent OpenTofu apply but does not propose any actions to undo any changes made outside of OpenTofu.`)
	cli.StringArrayVar(&targetsRaw, "target", nil,
		`Limit the planning operation to only the given module, resource, or resource instance and all of its dependencies. You can use this option multiple times to include more than one object. This is for exceptional use only. Cannot be used alongside the -exclude option.`,
	).SetDisplay("=resource")
	cli.StringArrayVar(&targetsFilesRaw, "target-file", nil,
		`Similar to -target, but specifies zero or more resource addresses from a file.`,
	).SetDisplay("=filename")
	cli.StringArrayVar(&excludesRaw, "exclude", nil,
		`Limit the planning operation to not operate on the given module, resource, or resource instance and all of the resources and modules that depend on it. You can use this option multiple times to exclude more than one object. This is for exceptional use only. Cannot be used together with the -target option.`,
	).SetDisplay("=resource")
	cli.StringArrayVar(&excludesFilesRaw, "exclude-file", nil,
		`Similar to -exclude, but specifies zero or more resource addresses from a file.`,
	).SetDisplay("=filename")
	cli.StringArrayVar(&forceReplaceRaw, "replace", nil,
		`Force replacement of a particular resource instance using its resource address. If the plan would've otherwise produced an update or no-op action for this instance, OpenTofu will plan to replace it instead. You can use this option multiple times to replace more than one object.`,
	).SetDisplay("=resource")

	// This processes the raw target flags into addrs.Targetable values, returning diagnostics if invalid.
	cli.PreHook(func() tfdiags.Diagnostics {
		var diags tfdiags.Diagnostics

		var parseDiags tfdiags.Diagnostics
		o.Targets, o.Excludes, parseDiags = parseRawTargetsAndExcludes(targetsRaw, excludesRaw, targetsFilesRaw, excludesFilesRaw)
		diags = diags.Append(parseDiags)

		for _, raw := range forceReplaceRaw {
			traversal, syntaxDiags := hclsyntax.ParseTraversalAbs([]byte(raw), "", hcl.Pos{Line: 1, Column: 1})
			if syntaxDiags.HasErrors() {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					fmt.Sprintf("Invalid force-replace address %q", raw),
					syntaxDiags[0].Detail,
				))
				continue
			}

			addr, addrDiags := addrs.ParseAbsResourceInstance(traversal)
			if addrDiags.HasErrors() {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					fmt.Sprintf("Invalid force-replace address %q", raw),
					addrDiags[0].Description().Detail,
				))
				continue
			}

			if addr.Resource.Resource.Mode != addrs.ManagedResourceMode {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					fmt.Sprintf("Invalid force-replace address %q", raw),
					"Only managed resources can be used with the -replace=... option.",
				))
				continue
			}

			o.ForceReplace = append(o.ForceReplace, addr)
		}

		// If you add a new possible value for o.PlanMode here, consider also
		// adding a specialized error message for it in ParseApplyDestroy.
		switch {
		case destroyRaw && refreshOnlyRaw:
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Incompatible plan mode options",
				"The -destroy and -refresh-only options are mutually-exclusive.",
			))
		case destroyRaw:
			o.PlanMode = plans.DestroyMode
		case refreshOnlyRaw:
			o.PlanMode = plans.RefreshOnlyMode
			if !o.Refresh {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Incompatible refresh options",
					"It doesn't make sense to use -refresh-only at the same time as -refresh=false, because OpenTofu would have nothing to do.",
				))
			}
		default:
			o.PlanMode = plans.NormalMode
		}

		return diags

	})

	return &o
}

// Vars describes arguments which specify non-default variable values. This
// interface is unfortunately obscure, because the order of the CLI arguments
// determines the final value of the gathered variables. In future it might be
// desirable for the arguments package to handle the gathering of variables
// directly, returning a map of variable values.
type Vars []flags.RawFlag

func (v Vars) All() Vars {
	return v
}

func (v Vars) Empty() bool {
	return len(v) == 0
}

// bind registers all Vars flags
func BindVars(cli *CommandLine) *Vars {
	varsFlags := flags.NewRawFlags("-var")
	varFilesFlags := varsFlags.Alias("-var-file")
	cli.RawFlags(varsFlags, "var",
		`Set a value for one of the input variables in the root module of the configuration. Use this option more than once to set more than one variable.`,
	).SetDisplay(" 'foo=bar'")
	cli.RawFlags(varFilesFlags, "var-file",
		`Load variable values from the given file, in addition to the default files terraform.tfvars and *.auto.tfvars. Use this option more than once to include more than one variables file.`,
	).SetDisplay("=filename")

	vars := &Vars{}
	cli.Vars = vars
	cli.PreHook(func() tfdiags.Diagnostics {
		if !varsFlags.Empty() {
			*vars = varsFlags.AllItems()
		}
		return nil
	})

	return vars
}
