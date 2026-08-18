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
	// View represents the global view options
	View *View

	// Vars holds and provides information for the flags related to variables that a user can give into the process
	Vars *Vars
}

// BindProvidersSchema registers CLI arguments, returning a ProvidersSchema value and it's corresponding hooks.
func BindProvidersSchema(cli *CommandLine) *ProvidersSchema {
	schema := ProvidersSchema{
		View: BindView(cli, viewFlagJson),
		Vars: BindVars(cli),
	}

	cli.PreHook(func() tfdiags.Diagnostics {
		if schema.View.ViewType != ViewJSON {
			return tfdiags.New(tfdiags.Sourceless(
				tfdiags.Error,
				"Output only in json is allowed",
				"The `tofu providers schema` command requires the `-json` flag.",
			))
		}

		// The 'providers schema' command just forces the user to use the `-json` flag but any of the diagnostics should
		// be printed as human format.
		// The print of the schema will be in JSON all the time.
		schema.View.ViewType = ViewHuman

		return nil
	})

	return &schema
}

// ParseProvidersSchema processes CLI arguments, returning a ProvidersSchema value, a closer function, and errors.
// If errors are encountered, a ProvidersSchema value is still returned representing
// the best effort interpretation of the arguments.
func ParseProvidersSchema(args []string) (*ProvidersSchema, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	schema := BindProvidersSchema(cli)
	closer, diags := cli.parseWithHooks("providers schema", args)
	return schema, closer, diags
}
