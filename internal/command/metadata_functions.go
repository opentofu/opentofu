// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views"
)

func MetadataFunctionsCommander() Command {
	cmd := Command{
		Name:  "functions",
		Short: "Show signatures and descriptions for the available functions",
		Long:  `Prints out a json representation of the available function signatures.`,
	}

	arguments.BindMetadataFunctions(&cmd.CommandLine)
	cmd.Run = func(meta Meta) int {
		view := views.NewMetadataFunctions(meta.View)
		if !view.PrintFunctions() {
			return 1
		}
		return 0
	}

	return cmd
}
