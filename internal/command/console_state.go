// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

type consoleBracketState struct {
	openNewLine int
	brace       int
	bracket     int
	parentheses int
	buffer      []string
}

// commandInOpenState return an int to inform if brackets are open
// or if any escaped new lines
// in the console and we should hold off on processing the commands
// it returns 3 states:
// -1 is returned the is an incorrect amount of brackets.
// for example "())" has too many close brackets
// 0 is returned if the brackets are closed.
// for examples "()" or "" would be in a close bracket state
// >=1 is returned for the amount of open brackets.
// for example "({" would return 2. "({}" would return 1
func (c *consoleBracketState) commandInOpenState() int {
	switch {
	case c.brace < 0:
		fallthrough
	case c.bracket < 0:
		fallthrough
	case c.parentheses < 0:
		return -1
	}

	// we calculate open brackets, braces and parentheses by the diff between each count
	var total int
	total += c.openNewLine
	total += c.brace
	total += c.bracket
	total += c.parentheses
	return total
}

// UpdateState updates the state of the console with the latest line data
func (c *consoleBracketState) UpdateState(line string) (string, int) {
	defer c.checkStateAndClearBuffer()
	// as new lines are a kind of "one off" we reset each update
	c.openNewLine = 0

	// escaped new lines are treated as a "one off" bracket
	// the four \\\\ means we have a false positive for a new line, as it's just an escaped \..
	if strings.HasSuffix(line, "\\") && !strings.HasSuffix(line, "\\\\") {
		c.openNewLine++
	}

	line = strings.TrimSuffix(line, "\\")
	if len(line) == 0 {
		// we can skip empty lines
		return c.getCommand(), c.commandInOpenState()
	}
	c.buffer = append(c.buffer, line)

	tokens, _ := hclsyntax.LexConfig([]byte(line), "<console-input>", hcl.Pos{Line: 1, Column: 1})
	for _, token := range tokens {
		switch token.Type { //nolint:exhaustive // we only care about these specific types
		case hclsyntax.TokenOBrace:
			c.brace++
		case hclsyntax.TokenCBrace:
			c.brace--
		case hclsyntax.TokenOBrack:
			c.bracket++
		case hclsyntax.TokenCBrack:
			c.bracket--
		case hclsyntax.TokenOParen:
			c.parentheses++
		case hclsyntax.TokenCParen:
			c.parentheses--
		}
	}
	return c.getCommand(), c.commandInOpenState()
}

// getCommand joins the buffer and returns it
func (c *consoleBracketState) getCommand() string {
	output := strings.Join(c.buffer, "\n")
	return output
}

func (c *consoleBracketState) checkStateAndClearBuffer() {
	if c.commandInOpenState() <= 0 {
		c.buffer = []string{}
	}
}
