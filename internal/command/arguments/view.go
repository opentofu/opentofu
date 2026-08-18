// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

// View represents the global command-line arguments which configure the view.
type View struct {
	// NoColor is used to disable the use of terminal color codes in all
	// output.
	NoColor bool

	// CompactWarnings is used to coalesce duplicate warnings, to reduce the
	// level of noise when multiple instances of the same warning are raised
	// for a configuration.
	CompactWarnings     bool
	ConsolidateWarnings bool
	ConsolidateErrors   bool

	// Concise is used to reduce the level of noise in the output and display
	// only the important details.
	Concise bool

	// ModuleDeprecationWarnLvl is used to filter out deprecation warnings for outputs and variables as requested by the user.
	ModuleDeprecationWarnLvl DeprecationWarningLevel

	// ShowSensitive is used to display the value of variables marked as sensitive.
	ShowSensitive bool

	// ===== Formerly ViewOptions ==== //
	// ViewOptions contains all of the information nessesary for constructing a view
	// from raw CLI arguments. This replaced most of the direct usage of ViewType
	// when the -json-into flag was introduced. In practice, this allows a much
	// more nuanced set of data to be presented to the view constructors.

	// ViewType specifies which output format to use
	ViewType ViewType

	// InputEnabled is used to disable interactive input for unspecified
	// variable and backend config values. Default is true.
	InputEnabled bool

	// Optional stream to write json data to
	JSONInto *os.File
}

// ViewType represents which view layer to use for a given command. Not all
// commands will support all view types, and validation that the type is
// supported should happen in the view constructor.
type ViewType rune

const (
	ViewNone  ViewType = 0
	ViewHuman ViewType = 'H'
	ViewJSON  ViewType = 'J'
	ViewRaw   ViewType = 'R'
)

func (vt ViewType) String() string {
	switch vt {
	case ViewNone:
		return "none"
	case ViewHuman:
		return "human"
	case ViewJSON:
		return "json"
	case ViewRaw:
		return "raw"
	default:
		return "unknown"
	}
}

type viewFlag uint8

const (
	viewFlagNone viewFlag = 0
	viewFlagJson viewFlag = 1 << iota
	viewFlagJsonInto
	viewFlagInput
	viewFlagSensitive

	viewFlagNoInput     = viewFlagJson | viewFlagJsonInto
	viewFlagNoSensitive = viewFlagNoInput | viewFlagInput
	viewFlagAll         = viewFlagNoSensitive | viewFlagSensitive
)

func BindView(cli *CommandLine, mask viewFlag) *View {
	var v View

	// This is a bit of a hack so we can correctly report diagnostics before actually executing the command
	cli.View = &v

	// SetGlobal here allows us to specify that the flag can be interspersed with positional arguments, something that the stdlib does *not* support.
	cli.BoolVar(&v.NoColor, "no-color", false, "Disable virtual terminal escape sequences.").SetGlobal(true)
	cli.BoolVar(&v.CompactWarnings, "compact-warnings", false, "If OpenTofu produces any warnings that are not accompanied by errors, shows them in a more compact form that includes only the summary messages.").SetGlobal(true)
	cli.BoolVar(&v.ConsolidateWarnings, "consolidate-warnings", true, "If OpenTofu produces any warnings, no consolidation will be performed. All locations, for all warnings will be listed. Enabled by default.").SetDisplay("=false").SetGlobal(true)
	cli.BoolVar(&v.ConsolidateErrors, "consolidate-errors", false, "If OpenTofu produces any errors, no consolidation will be performed. All locations, for all errors will be listed. Disabled by default.").SetGlobal(true)
	cli.BoolVar(&v.Concise, "concise", false, "Disable progress-related messages.").SetGlobal(true)

	if mask&viewFlagSensitive != 0 {
		cli.BoolVar(&v.ShowSensitive, "show-sensitive", false, "If specified, sensitive values will be displayed.")
	}

	var deprecation []string
	cli.StringArrayVar(&deprecation, "deprecation", nil, `Specify what type of warnings are shown. Accepted values for "m": all, local, none. Default: all. When "all" is selected, OpenTofu will show the deprecation warnings for all modules. When "local" is selected, the warns will be shown only for the modules that are imported with a relative path. When "none" is selected, all the deprecation warnings will be dropped.`).SetDisplay("=module:m").SetGlobal(true)

	cli.PreHook(func() tfdiags.Diagnostics {
		var diags tfdiags.Diagnostics

		for _, s := range deprecation {
			prefix := "module:"
			if len(deprecation) != 0 && !strings.HasPrefix(s, prefix) {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Invalid argument",
					fmt.Sprintf("Expected -deprecation prefix %q, got %q", prefix, s),
				))
				continue
			}
			v.ModuleDeprecationWarnLvl = ParseDeprecatedWarningLevel(strings.ReplaceAll(s, prefix, ""))
		}

		return diags
	})

	// View Options
	var jsonFlag bool
	if mask&viewFlagJson != 0 {
		cli.BoolVar(&jsonFlag, "json", false, `Produce output in a machine-readable JSON format, suitable for use in text editor integrations and other automated systems.`)
	}
	var jsonIntoFlag string
	if mask&viewFlagJsonInto != 0 {
		cli.StringVar(&jsonIntoFlag, "json-into", "", `Produce the same output as -json, but sent directly to the given file. This allows automation to preserve the original human-readable output streams, while capturing more detailed logs for machine analysis.`).SetDisplay("=out.json")
	}
	if mask&viewFlagInput != 0 {
		cli.BoolVar(&v.InputEnabled, "input", true, `Disable prompting for required input variables that are not set some other way.`).SetDisplay("=false")
	}

	closer := func() {}
	cli.PreHook(func() tfdiags.Diagnostics {
		var diags tfdiags.Diagnostics

		if jsonIntoFlag != "" {
			// Although it seems odd to add complex logic to the arguments
			// package, it is currently the most reasonable place for this
			// particular concern. The only other reasonable spot currently
			// in the codebase is within the view constructor. Unfortunately
			// that is not an option due to command code paths opening
			// multiple concurrent views.
			v.JSONInto, closer, diags = openJsonIntoFile(jsonIntoFlag)
		}

		// Default to Human
		v.ViewType = ViewHuman
		if jsonFlag {
			v.ViewType = ViewJSON
			// JSON view currently does not support input, so we disable it here
			v.InputEnabled = false
			if jsonIntoFlag != "" {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Invalid output format",
					"The -json and -json-into arguments are mutually exclusive",
				))
			}
		}
		return diags
	})
	cli.PostHook(func() tfdiags.Diagnostics {
		closer()
		return nil
	})

	return &v
}

func openJsonIntoFile(jsonIntoFlag string) (*os.File, func(), tfdiags.Diagnostics) {
	closer := func() {}
	var diags tfdiags.Diagnostics

	JSONInto, err := os.OpenFile(jsonIntoFlag, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid argument",
			fmt.Sprintf("Unable to open the file %q specified by -json-into for writing: %s", jsonIntoFlag, err.Error()),
		))
	} else {
		closer = func() {
			err := JSONInto.Close()
			if err != nil {
				log.Printf("[ERROR] Unable to close json output: %s", err.Error())
			}
		}
	}

	return JSONInto, closer, diags
}
