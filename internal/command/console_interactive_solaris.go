// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build solaris
// +build solaris

package command

import (
	"fmt"

	"github.com/mitchellh/cli"

	"github.com/opentofu/opentofu/internal/repl"
)

func (c *ConsoleCommand) modeInteractive(session *repl.Session, ui cli.Ui) int {
	ui.Error(fmt.Sprintf(
		"The readline library OpenTofu currently uses for the interactive\n" +
			"console is not supported by Solaris. Interactive mode is therefore\n" +
			"not supported on Solaris currently."))
	return 1
}
