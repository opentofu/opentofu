// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package lsp

import (
	"bytes"
	"context"
	"strings"

	"github.com/opentofu/opentofu/internal/tofufmt"
	"go.lsp.dev/protocol"
)

func (s *server) Formatting(ctx context.Context, params *protocol.DocumentFormattingParams) ([]protocol.TextEdit, error) {
	uri := params.TextDocument.URI

	s.mu.Lock()
	src, ok := s.docs[uri]
	s.mu.Unlock()

	if !ok {
		return nil, nil
	}

	filename, err := pathFromURI(uri)
	if err != nil {
		filename = string(uri)
	}

	formatted, diags := tofufmt.Format(src, filename)

	if diags.HasErrors() {
		// dont error here, just return nil, nil so that the LSP thinks
		// nothing needs changing
		return nil, nil
	}

	if bytes.Equal(formatted, src) {
		return []protocol.TextEdit{}, nil
	}

	// count the lines
	lineCount := uint32(strings.Count(string(src), "\n") + 1)

	return []protocol.TextEdit{
		{
			Range: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End:   protocol.Position{Line: lineCount, Character: 0},
			},
			NewText: string(formatted),
		},
	}, nil
}
