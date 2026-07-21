// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"

	"github.com/mitchellh/cli"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type LsCommand struct {
	Meta
}

var _ cli.Command = LsCommand{}

func (c LsCommand) Help() string {

	return ""
}
func (c LsCommand) Synopsis() string {
	return ""
}

func (c LsCommand) Run(args []string) int {
	cmdFlags := flag.NewFlagSet("ls", flag.ContinueOnError)
	cmdFlags.SetOutput(io.Discard)
	// TODO: Move this out to its own method, add usage
	err := cmdFlags.Parse(args)
	if err != nil {
		var diags tfdiags.Diagnostics

		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
		c.View.Diagnostics(diags)
	}

	// TODO: arg error handling

	ctx, done := c.InterruptibleContext(c.CommandContext())
	defer done()

	err = lsp.Serve(ctx, stdioConn{})
	if err != nil && errors.Is(err, context.Canceled) {
		var diags tfdiags.Diagnostics

		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Language server error",
			err.Error(),
		))
		c.View.Diagnostics(diags)
	}

	return 0
}

type stdioConn struct{}

var _ io.ReadWriteCloser = stdioConn{}

func (s stdioConn) Close() error {
	return os.Stdin.Close()
}

func (s stdioConn) Write(p []byte) (n int, err error) {
	return os.Stdout.Write(p)
}

func (s stdioConn) Read(p []byte) (n int, err error) {
	return os.Stdin.Read(p)
}
