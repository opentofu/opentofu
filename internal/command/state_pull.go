// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/states/statefile"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

// StatePullCommand is a Command implementation that shows a single resource.
type StatePullCommand struct {
	Meta
	StateMeta
}

func (c *StatePullCommand) Run(args []string) int {
	ctx := c.CommandContext()

	args = c.Meta.process(args)
	cmdFlags := c.Meta.defaultFlagSet("state pull")
	c.Meta.varFlagSet(cmdFlags)
	if err := cmdFlags.Parse(args); err != nil {
		c.Ui.Error(fmt.Sprintf("Error parsing command-line flags: %s\n", err.Error()))
		return 1
	}

	if diags := c.Meta.checkRequiredVersion(ctx); diags != nil {
		c.showDiagnostics(diags)
		return 1
	}

	// Load the encryption configuration
	enc, encDiags := c.Encryption(ctx)
	if encDiags.HasErrors() {
		c.showDiagnostics(encDiags)
		return 1
	}

	// Load the backend
	b, backendDiags := c.Backend(ctx, nil, enc.State())
	if backendDiags.HasErrors() {
		c.showDiagnostics(backendDiags)
		return 1
	}

	// This is a read-only command
	c.ignoreRemoteVersionConflict(b)

	// Get the state manager for the current workspace
	env, err := c.Workspace(ctx)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error selecting workspace: %s", err))
		return 1
	}
	stateMgr, err := b.StateMgr(ctx, env)
	if err != nil {
		c.Ui.Error(fmt.Sprintf(errStateLoadingState, err))
		return 1
	}
	if err := stateMgr.RefreshState(context.TODO()); err != nil {
		c.Ui.Error(fmt.Sprintf("Failed to refresh state: %s", err))
		return 1
	}

	// Get a statefile object representing the latest snapshot
	stateFile := statemgr.Export(stateMgr)

	if stateFile != nil { // we produce no output if the statefile is nil
		var buf bytes.Buffer
		err = statefile.Write(stateFile, &buf, encryption.StateEncryptionDisabled()) // Don't encrypt to stdout
		if err != nil {
			c.Ui.Error(fmt.Sprintf("Failed to write state: %s", err))
			return 1
		}

		c.Ui.Output(buf.String())
	}

	return 0
}

func (c *StatePullCommand) Help() string {
	helpText := `
Usage: tofu [global options] state pull [options]

  Pull the state from its location, upgrade the local copy, and output it
  to stdout.

  This command "pulls" the current state and outputs it to stdout.
  As part of this process, OpenTofu will upgrade the state format of the
  local copy to the current version.

  The primary use of this is for state stored remotely. This command
  will still work with local state but is less useful for this.

Options:

  -var 'foo=bar'     Set a value for one of the input variables in the root
                     module of the configuration. Use this option more than
                     once to set more than one variable.

  -var-file=filename Load variable values from the given file, in addition
                     to the default files terraform.tfvars and *.auto.tfvars.
                     Use this option more than once to include more than one
                     variables file.
`
	return strings.TrimSpace(helpText)
}

func (c *StatePullCommand) Synopsis() string {
	return "Pull current state and output to stdout"
}
