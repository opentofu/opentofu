// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ProvidersSchema represents the command-line arguments for the 'providers schema' command.
type ProvidersSchema struct {
	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions

	// Vars holds and provides information for the flags related to variables that a user can give into the process
	Vars *Vars
}

// ParseProvidersSchema processes CLI arguments, returning a ProvidersSchema value, a closer function, and errors.
// If errors are encountered, a ProvidersSchema value is still returned representing
// the best effort interpretation of the arguments.
func ParseProvidersSchema(args []string) (*ProvidersSchema, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	schema := &ProvidersSchema{
		Vars: &Vars{},
	}

	cmdFlags := extendedFlagSet("providers schema", nil, nil, schema.Vars)

	schema.ViewOptions.AddGranularFlags(cmdFlags, false, false)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	args = cmdFlags.Args()
	if len(args) > 0 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Too many command line arguments",
			"Expected at most zero positional arguments.",
		))
	}

	closer, moreDiags := schema.ViewOptions.Parse()
	diags = diags.Append(moreDiags)

	if schema.ViewOptions.ViewType != ViewJSON {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Output only in json is allowed",
			"The `tofu providers schema` command requires the `-json` flag.",
		))
	}

	return schema, closer, diags
}
