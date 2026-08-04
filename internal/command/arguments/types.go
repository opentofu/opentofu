// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"fmt"
	"log"
	"os"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

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

// ViewOptions contains all of the information nessesary for constructing a view
// from raw CLI arguments. This replaced most of the direct usage of ViewType
// when the -json-into flag was introduced. In practice, this allows a much
// more nuanced set of data to be presented to the view constructors.
type ViewOptions struct {
	// Raw cli flags
	jsonFlag     bool
	jsonIntoFlag string

	// ViewType specifies which output format to use
	ViewType ViewType

	// InputEnabled is used to disable interactive input for unspecified
	// variable and backend config values. Default is true.
	InputEnabled bool

	// Optional stream to write json data to
	JSONInto *os.File
}

func (v *ViewOptions) bind(cli *CommandLine, input bool) {
	v.bindGranularFlags(cli, input, true)
}

// bindGranularFlags registers view-related flags on cli. Use input=true to
// register the -input flag and jsonInto=true to register the -json-into flag.
// Commands that only support -json (not -json-into) should pass jsonInto=false.
func (v *ViewOptions) bindGranularFlags(cli *CommandLine, input bool, jsonInto bool) {
	if input {
		cli.BoolVar(&v.InputEnabled, "input", true,
			`Disable prompting for required input variables that are not set some other way.`,
		).SetDisplay("=false")
	}

	cli.BoolVar(&v.jsonFlag, "json", false,
		`Produce output in a machine-readable JSON format, suitable for use in text editor integrations and other automated systems.`)
	if jsonInto {
		cli.StringVar(&v.jsonIntoFlag, "json-into", "",
			`Produce the same output as -json, but sent directly to the given file. This allows automation to preserve the original human-readable output streams, while capturing more detailed logs for machine analysis.`,
		).SetDisplay("=out.json")
	}

	v.ParseHook(cli)
}

func (v *ViewOptions) ParseHook(cli *CommandLine) {
	closer := func() {}
	cli.Hook(Hook{Pre: func() tfdiags.Diagnostics {
		var diags tfdiags.Diagnostics

		if v.jsonIntoFlag != "" {
			// Although it seems odd to add complex logic to the arguments
			// package, it is currently the most reasonable place for this
			// particular concern. The only other reasonable spot currently
			// in the codebase is within the view constructor. Unfortunately
			// that is not an option due to command code paths opening
			// multiple concurrent views.
			v.JSONInto, closer, diags = OpenJSONIntoFile(v.jsonIntoFlag)
		}

		// Default to Human
		v.ViewType = ViewHuman
		if v.jsonFlag {
			v.ViewType = ViewJSON
			// JSON view currently does not support input, so we disable it here
			v.InputEnabled = false
			if v.jsonIntoFlag != "" {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Invalid output format",
					"The -json and -json-into arguments are mutually exclusive",
				))
			}
		}
		return diags
	}, Post: func() tfdiags.Diagnostics {
		closer()
		return nil
	}})
}

func OpenJSONIntoFile(jsonIntoFlag string) (*os.File, func(), tfdiags.Diagnostics) {
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
