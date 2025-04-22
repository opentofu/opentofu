// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// DefaultParallelism is the limit OpenTofu places on total parallel
// operations as it walks the dependency graph.
const DefaultParallelism = 10

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
	StatePath string

	// StateOutPath specifies a different path to write the final state file.
	// The default value is blank, which results in state being written back to
	// StatePath.
	StateOutPath string

	// BackupPath specifies the path where a backup copy of the state file will
	// be stored before the new state is written. The default value is blank,
	// which is interpreted as StateOutPath +
	// ".backup".
	BackupPath string
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

	// These private fields are used only temporarily during decoding. Use
	// method Parse to populate the exported fields from these, validating
	// the raw values in the process.
	targetsRaw       []string
	targetsFilesRaw  []string
	excludesRaw      []string
	excludesFilesRaw []string
	forceReplaceRaw  []string
	destroyRaw       bool
	refreshOnlyRaw   bool
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

// Parse must be called on Operation after initial flag parse. This processes
// the raw target flags into addrs.Targetable values, returning diagnostics if
// invalid.
func (o *Operation) Parse() tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	var parseDiags tfdiags.Diagnostics
	o.Targets, o.Excludes, parseDiags = parseRawTargetsAndExcludes(o.targetsRaw, o.excludesRaw, o.targetsFilesRaw, o.excludesFilesRaw)
	diags = diags.Append(parseDiags)

	for _, raw := range o.forceReplaceRaw {
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
	case o.destroyRaw && o.refreshOnlyRaw:
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Incompatible plan mode options",
			"The -destroy and -refresh-only options are mutually-exclusive.",
		))
	case o.destroyRaw:
		o.PlanMode = plans.DestroyMode
	case o.refreshOnlyRaw:
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
}

// Vars describes arguments which specify non-default variable values. This
// interface is unfortunately obscure, because the order of the CLI arguments
// determines the final value of the gathered variables. In future it might be
// desirable for the arguments package to handle the gathering of variables
// directly, returning a map of variable values.
type Vars struct {
	vars     *flagNameValueSlice
	varFiles *flagNameValueSlice
}

func (v *Vars) All() []FlagNameValue {
	if v.vars == nil {
		return nil
	}
	return v.vars.AllItems()
}

func (v *Vars) Empty() bool {
	if v.vars == nil {
		return true
	}
	return v.vars.Empty()
}

// extendedFlagSet creates a FlagSet with common backend, operation, and vars
// flags used in many commands. Target structs for each subset of flags must be
// provided in order to support those flags.
func extendedFlagSet(name string, state *State, operation *Operation, vars *Vars) *flag.FlagSet {
	f := defaultFlagSet(name)

	if state == nil && operation == nil && vars == nil {
		panic("use defaultFlagSet")
	}

	if state != nil {
		f.BoolVar(&state.Lock, "lock", true, "lock")
		f.DurationVar(&state.LockTimeout, "lock-timeout", 0, "lock-timeout")
		f.StringVar(&state.StatePath, "state", "", "state-path")
		f.StringVar(&state.StateOutPath, "state-out", "", "state-path")
		f.StringVar(&state.BackupPath, "backup", "", "backup-path")
	}

	if operation != nil {
		f.IntVar(&operation.Parallelism, "parallelism", DefaultParallelism, "parallelism")
		f.BoolVar(&operation.Refresh, "refresh", true, "refresh")
		f.BoolVar(&operation.destroyRaw, "destroy", false, "destroy")
		f.BoolVar(&operation.refreshOnlyRaw, "refresh-only", false, "refresh-only")
		f.Var((*flagStringSlice)(&operation.targetsRaw), "target", "target")
		f.Var((*flagStringSlice)(&operation.targetsFilesRaw), "target-file", "target-file")
		f.Var((*flagStringSlice)(&operation.excludesRaw), "exclude", "exclude")
		f.Var((*flagStringSlice)(&operation.excludesFilesRaw), "exclude-file", "exclude-file")
		f.Var((*flagStringSlice)(&operation.forceReplaceRaw), "replace", "replace")
	}

	// Gather all -var and -var-file arguments into one heterogeneous structure
	// to preserve the overall order.
	if vars != nil {
		varsFlags := newFlagNameValueSlice("-var")
		varFilesFlags := varsFlags.Alias("-var-file")
		vars.vars = &varsFlags
		vars.varFiles = &varFilesFlags
		f.Var(vars.vars, "var", "var")
		f.Var(vars.varFiles, "var-file", "var-file")
	}

	return f
}
