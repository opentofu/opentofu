// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

type consoleBracketState struct {
	oBrace       int
	cBrace       int
	oBracket     int
	cBracket     int
	oParentheses int
	cParentheses int
}

// BracketsOpen return an int to inform if brackets are open
// in the console and we should hold off on processing the commands
// it returns 3 states:
// -1 is returned the is an incorrect amount of brackets.
// for example "())" has too many close brackets
// 0 is returned if the brackets are closed.
// for examples "()" or "" would be in a close bracket state
// >=1 is returned for the amount of open brackets.
// for example "({" would return 2. "({}" would return 1
func (c *consoleBracketState) BracketsOpen() int {
	switch {
	case c.oBrace < c.cBrace:
		fallthrough
	case c.oBracket < c.cBracket:
		fallthrough
	case c.oParentheses < c.cParentheses:
		return -1
	}

	// we calculate open brackets, braces and parentheses by the diff between each count
	var total int
	total += c.oBrace - c.cBrace
	total += c.oBracket - c.cBracket
	total += c.oParentheses - c.cParentheses
	return total
}

// UpdateState updates the state of the console with the latest line data
func (c *consoleBracketState) UpdateState(line string) {
	tokens, _ := hclsyntax.LexConfig([]byte(line), "<console-input>", hcl.Pos{Line: 1, Column: 1})
	for _, token := range tokens {
		switch token.Type {
		case hclsyntax.TokenOBrace:
			c.oBrace++
		case hclsyntax.TokenCBrace:
			c.cBrace++
		case hclsyntax.TokenOBrack:
			c.oBracket++
		case hclsyntax.TokenCBrack:
			c.cBracket++
		case hclsyntax.TokenOParen:
			c.oParentheses++
		case hclsyntax.TokenCParen:
			c.cParentheses++
		// we don't care about these types, but the linter doesn't like it if we don't mention them, so we just NOOP on them
		case hclsyntax.TokenOQuote, hclsyntax.TokenCQuote, hclsyntax.TokenOHeredoc, hclsyntax.TokenCHeredoc, hclsyntax.TokenStar, hclsyntax.TokenSlash, hclsyntax.TokenPlus, hclsyntax.TokenMinus, hclsyntax.TokenPercent, hclsyntax.TokenEqual, hclsyntax.TokenEqualOp, hclsyntax.TokenNotEqual, hclsyntax.TokenLessThan, hclsyntax.TokenLessThanEq, hclsyntax.TokenGreaterThan, hclsyntax.TokenGreaterThanEq, hclsyntax.TokenAnd, hclsyntax.TokenOr, hclsyntax.TokenBang, hclsyntax.TokenDot, hclsyntax.TokenComma, hclsyntax.TokenDoubleColon, hclsyntax.TokenEllipsis, hclsyntax.TokenFatArrow, hclsyntax.TokenQuestion, hclsyntax.TokenColon, hclsyntax.TokenTemplateInterp, hclsyntax.TokenTemplateControl, hclsyntax.TokenTemplateSeqEnd, hclsyntax.TokenQuotedLit, hclsyntax.TokenStringLit, hclsyntax.TokenNumberLit, hclsyntax.TokenIdent, hclsyntax.TokenComment, hclsyntax.TokenNewline, hclsyntax.TokenEOF, hclsyntax.TokenBitwiseAnd, hclsyntax.TokenBitwiseOr, hclsyntax.TokenBitwiseNot, hclsyntax.TokenBitwiseXor, hclsyntax.TokenStarStar, hclsyntax.TokenApostrophe, hclsyntax.TokenBacktick, hclsyntax.TokenSemicolon, hclsyntax.TokenTabs, hclsyntax.TokenInvalid, hclsyntax.TokenBadUTF8, hclsyntax.TokenQuotedNewline, hclsyntax.TokenNil:
		default:
		}
	}
}

// ClearState is used to reset the state after an evaluation
func (c *consoleBracketState) ClearState() {
	c.oBrace = 0
	c.cBrace = 0
	c.oBracket = 0
	c.cBracket = 0
	c.oParentheses = 0
	c.cParentheses = 0
}
