// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/opentofu/opentofu/internal/repl"

	"github.com/chzyer/readline"
	"github.com/mitchellh/cli"
)

func (c *ConsoleCommand) modeInteractive(session *repl.Session, ui cli.Ui) int {
	// Configure input
	l, err := readline.NewEx(&readline.Config{
		Prompt:            "> ",
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
		HistorySearchFold: true,
		Stdin:             os.Stdin,
		Stdout:            os.Stdout,
		Stderr:            os.Stderr,
	})
	if err != nil {
		c.Ui.Error(fmt.Sprintf(
			"Error initializing console: %s",
			err))
		return 1
	}
	defer l.Close()

	var consoleState consoleBracketState

	for {
		// Read a line
		line, err := l.Readline()
		if errors.Is(err, readline.ErrInterrupt) {
			if len(line) == 0 {
				break
			} else {
				continue
			}
		} else if errors.Is(err, io.EOF) {
			break
		}
		line = strings.TrimSpace(line)

		// we update the state with the new line, so if we have open
		// brackets we know not to execute the command just yet
		fullCommand, openState := consoleState.UpdateState(line)

		switch {
		case openState > 0:
			// here there are open brackets somewhere, so we don't execute it
			// as we are in a bracket we update the prompt. we use one . per layer pf brackets
			l.SetPrompt(fmt.Sprintf("%s ", strings.Repeat(".", openState)))
		default:
			out, exit, diags := session.Handle(fullCommand)
			if diags.HasErrors() {
				c.showDiagnostics(diags)
			}
			if exit {
				break
			}

			// clear the state and buffer as we have executed a command
			// we also reset the prompt
			l.SetPrompt("> ")

			ui.Output(out)
		}
	}

	return 0
}
