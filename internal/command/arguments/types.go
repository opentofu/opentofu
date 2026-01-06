// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"flag"
	"fmt"
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

func (v *ViewOptions) AddFlags(cmdFlags *flag.FlagSet, input bool) {
	if input {
		cmdFlags.BoolVar(&v.InputEnabled, "input", true, "input")
	}

	cmdFlags.BoolVar(&v.jsonFlag, "json", false, "json")
	cmdFlags.StringVar(&v.jsonIntoFlag, "json-into", "", "json-into")
}

func (v *ViewOptions) Parse() (func() error, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	closer := func() error { return nil }

	if v.jsonIntoFlag != "" {
		var err error
		v.JSONInto, err = os.OpenFile(v.jsonIntoFlag, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0600)
		if err != nil {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid argument",
				fmt.Sprintf("Unable to open the file %q specified by -json-into for writing: %s", v.jsonIntoFlag, err.Error()),
			))
		} else {
			closer = v.JSONInto.Close
		}
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
	return closer, diags
}
