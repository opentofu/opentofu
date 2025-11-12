// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/mitchellh/colorstring"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/format"
	"github.com/opentofu/opentofu/internal/terminal"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu"
)

// View is the base layer for command views, encapsulating a set of I/O
// streams, a colorize implementation, and implementing a human friendly view
// for diagnostics.
type View struct {
	streams  *terminal.Streams
	colorize *colorstring.Colorize

	compactWarnings     bool
	consolidateWarnings bool
	consolidateErrors   bool

	// When this is true it's a hint that OpenTofu is being run indirectly
	// via a wrapper script or other automation and so we may wish to replace
	// direct examples of commands to run with more conceptual directions.
	// However, we only do this on a best-effort basis, typically prioritizing
	// the messages that users are most likely to see.
	runningInAutomation bool

	// Concise is used to reduce the level of noise in the output and display
	// only the important details.
	concise bool

	// ModuleDeprecationWarnLvl is used to filter out deprecation warnings for outputs and variables as requested by the user.
	ModuleDeprecationWarnLvl tofu.DeprecationWarningLevel

	// showSensitive is used to display the value of variables marked as sensitive.
	showSensitive bool

	// This unfortunate wart is required to enable rendering of diagnostics which
	// have associated source code in the configuration. This function pointer
	// will be dereferenced as late as possible when rendering diagnostics in
	// order to access the config loader cache.
	configSources func() map[string]*hcl.File
}

// Initialize a View with the given streams, a disabled colorize object, and a
// no-op configSources callback.
func NewView(streams *terminal.Streams) *View {
	return &View{
		streams: streams,
		colorize: &colorstring.Colorize{
			Colors:  colorstring.DefaultColors,
			Disable: true,
			Reset:   true,
		},
		configSources: func() map[string]*hcl.File { return nil },
	}
}

// SetRunningInAutomation modifies the view's "running in automation" flag,
// which causes some slight adjustments to certain messages that would normally
// suggest specific OpenTofu commands to run, to make more conceptual gestures
// instead for situations where the user isn't running OpenTofu directly.
//
// For convenient use during initialization (in conjunction with NewView),
// SetRunningInAutomation returns the receiver after modifying it.
func (v *View) SetRunningInAutomation(new bool) *View {
	v.runningInAutomation = new
	return v
}

func (v *View) RunningInAutomation() bool {
	return v.runningInAutomation
}

// Configure applies the global view configuration flags.
func (v *View) Configure(view *arguments.View) {
	v.colorize.Disable = view.NoColor
	v.compactWarnings = view.CompactWarnings
	v.consolidateWarnings = view.ConsolidateWarnings
	v.consolidateErrors = view.ConsolidateErrors
	v.concise = view.Concise
	v.ModuleDeprecationWarnLvl = view.ModuleDeprecationWarnLvl
}

// SetConfigSources overrides the default no-op callback with a new function
// pointer, and should be called when the config loader is initialized.
func (v *View) SetConfigSources(cb func() map[string]*hcl.File) {
	v.configSources = cb
}

// Diagnostics renders a set of warnings and errors in human-readable form.
// Warnings are printed to stdout, and errors to stderr.
func (v *View) Diagnostics(diags tfdiags.Diagnostics) {
	diags.Sort()

	if len(diags) == 0 {
		return
	}

	// Filter the deprecation warnings based on the cli arg.
	// For safety and performance reasons, we are filtering the deprecation related diagnostics only when
	// the filtering level is not tofu.DeprecationWarningLevelAll.
	// This filtering is implemented only in here, and not in meta.go#showDiagnostics because there are meant to be
	// shown only during apply and plan phases. These 2 phases are using this implementation to interact with the user
	// while meta.go#showDiagnostics is used by other commands that are not meant to show the deprecation diagnostics.
	if v.ModuleDeprecationWarnLvl != tofu.DeprecationWarningLevelAll {
		var newDiags tfdiags.Diagnostics
		for _, diag := range diags {
			if !tofu.DeprecationDiagnosticAllowed(v.ModuleDeprecationWarnLvl, diag) {
				continue
			}
			newDiags = append(newDiags, diag)
		}
		diags = newDiags
	}

	if v.consolidateWarnings {
		diags = diags.Consolidate(1, tfdiags.Warning)
	}
	if v.consolidateErrors {
		diags = diags.Consolidate(1, tfdiags.Error)
	}

	// Since warning messages are generally competing
	if v.compactWarnings {
		// If the user selected compact warnings and all of the diagnostics are
		// warnings then we'll use a more compact representation of the warnings
		// that only includes their summaries.
		// We show full warnings if there are also errors, because a warning
		// can sometimes serve as good context for a subsequent error.
		useCompact := true
		for _, diag := range diags {
			if diag.Severity() != tfdiags.Warning {
				useCompact = false
				break
			}
		}
		if useCompact {
			msg := format.DiagnosticWarningsCompact(diags, v.colorize)
			msg = "\n" + msg + "\nTo see the full warning notes, run OpenTofu without -compact-warnings.\n"
			v.streams.Print(msg)
			return
		}
	}

	for _, diag := range diags {
		var msg string
		if v.colorize.Disable {
			msg = format.DiagnosticPlain(diag, v.configSources(), v.streams.Stderr.Columns())
		} else {
			msg = format.Diagnostic(diag, v.configSources(), v.colorize, v.streams.Stderr.Columns())
		}

		if diag.Severity() == tfdiags.Error {
			v.streams.Eprint(msg)
		} else {
			v.streams.Print(msg)
		}
	}
}

// HelpPrompt is intended to be called from commands which fail to parse all
// of their CLI arguments successfully. It refers users to the full help output
// rather than rendering it directly, which can be overwhelming and confusing.
func (v *View) HelpPrompt(command string) {
	v.streams.Eprintf(helpPrompt, command)
}

const helpPrompt = `
For more help on using this command, run:
  tofu %s -help
`

// outputColumns returns the number of text character cells any non-error
// output should be wrapped to.
//
// This is the number of columns to use if you are calling v.streams.Print or
// related functions.
func (v *View) outputColumns() int {
	return v.streams.Stdout.Columns()
}

// errorColumns returns the number of text character cells any error
// output should be wrapped to.
//
// This is the number of columns to use if you are calling v.streams.Eprint
// or related functions.
func (v *View) errorColumns() int {
	return v.streams.Stderr.Columns()
}

// outputHorizRule will call v.streams.Println with enough horizontal line
// characters to fill an entire row of output.
//
// If UI color is enabled, the rule will get a dark grey coloring to try to
// visually de-emphasize it.
func (v *View) outputHorizRule() {
	v.streams.Println(format.HorizontalRule(v.colorize, v.outputColumns()))
}

func (v *View) SetShowSensitive(showSensitive bool) {
	v.showSensitive = showSensitive
}
