// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package lsp

import (
	"context"

	"go.lsp.dev/protocol"
)

func (s *server) DidOpen(ctx context.Context, params *protocol.DidOpenTextDocumentParams) error {
	u := params.TextDocument.URI
	src := []byte(params.TextDocument.Text)

	s.mu.Lock()
	s.docs[u] = src
	s.mu.Unlock()

	s.publishDiagnostics(ctx, u, src)

	return nil
}

func (s *server) DidChange(ctx context.Context, params *protocol.DidChangeTextDocumentParams) error {
	if len(params.ContentChanges) == 0 {
		return nil
	}

	u := params.TextDocument.URI

	latestChange := params.ContentChanges[len(params.ContentChanges)-1]

	var text string

	switch change := latestChange.(type) {
	case *protocol.TextDocumentContentChangeWholeDocument:
		text = change.Text
	case *protocol.TextDocumentContentChangePartial:
		// TODO: Handle better, right now replace it all because we can just sync with the change type whole
		text = change.Text
	default:
		return nil
	}

	src := []byte(text)

	s.mu.Lock()
	s.docs[u] = src
	s.mu.Unlock()

	s.publishDiagnostics(ctx, u, src) // TODO: error handling

	return nil
}

func (s *server) DidClose(ctx context.Context, params *protocol.DidCloseTextDocumentParams) error {

	u := params.TextDocument.URI

	s.mu.Lock()
	delete(s.docs, u)
	s.mu.Unlock()

	return nil
}
