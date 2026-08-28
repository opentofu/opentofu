// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

func MetadataCommander() Command {
	cmd := Command{
		Name:  "metadata",
		Short: "Metadata related commands",
		Long:  "This command has subcommands for metadata related purposes.",

		Commands: []Command{MetadataFunctionsCommander()},
	}

	return cmd
}
