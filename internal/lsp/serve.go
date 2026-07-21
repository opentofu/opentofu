// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package lsp

import (
	"context"
	"io"
	"sync"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

func Serve(ctx context.Context, rwc io.ReadWriteCloser) error {
	stream := jsonrpc2.NewStream(rwc)
	srv := newServer()

	_, conn, client := protocol.NewServer(ctx, srv, stream)
	srv.setClient(client)

	select {
	case <-ctx.Done():
		_ = conn.Close()
		return ctx.Err()
	case <-conn.Done():
		return conn.Err()
	}
}

type server struct {
	protocol.UnimplementedServer

	mu     sync.Mutex
	docs   map[uri.URI][]byte
	client protocol.Client
}

func newServer() *server {
	return &server{
		docs: make(map[uri.URI][]byte),
	}
}

func (s *server) setClient(client protocol.Client) {
	s.client = client
}

func (s *server) Initialize(ctx context.Context, params *protocol.InitializeParams) (*protocol.InitializeResult, error) {
	result := &protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			DocumentFormattingProvider: protocol.Boolean(true),
			TextDocumentSync:           protocol.TextDocumentSyncKindFull, // send us full files, not partial
		},
		ServerInfo: protocol.ServerInfo{
			Name: "tofu-ls",
		},
	}
	return result, nil
}
