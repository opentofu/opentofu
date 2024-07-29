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

	cmds := make([]string, 0, commandBufferIntitialSize)

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

		// we update the state with the new line, so if we have open
		// brackets we know not to execute the command just yet
		consoleState.UpdateState(line)

		switch {
		case len(line) == 0:
			// here we have an empty line, so can ignore
			continue
		case strings.HasSuffix(line, "\\"):
			// here the new line is escaped, so we fallthough to the open brackets case as
			// we handle them the same way
			line = strings.TrimSuffix(line, "\\")
			fallthrough
		case consoleState.BracketsOpen() > 0:
			// here there are open brackets somewhere, so we remember the command
			// but don't execute it
			// as we are in a bracket we update the prompt. we use one . per layer pf brackets
			l.SetPrompt(fmt.Sprintf("%s ", strings.Repeat(".", consoleState.BracketsOpen())))
			cmds = append(cmds, line)
		case consoleState.BracketsOpen() <= 0:
			// here we either have no more open brackets or an invalid amount of brackets
			// either way we fall through to execute the command and let hcl parse it
			fallthrough
		default:
			cmds = append(cmds, line)
			bigLine := strings.Join(cmds, "\n")
			out, exit, diags := session.Handle(bigLine)
			if diags.HasErrors() {
				c.showDiagnostics(diags)
			}
			if exit {
				break
			}

			// clear the state and buffer as we have executed a command
			// we also reset the prompt
			l.SetPrompt("> ")
			consoleState.ClearState()
			cmds = make([]string, 0, commandBufferIntitialSize)

			ui.Output(out)
		}
	}

	return 0
}
