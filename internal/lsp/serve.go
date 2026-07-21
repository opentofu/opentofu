package lsp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/opentofu/opentofu/internal/tofufmt"
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
		},
		ServerInfo: protocol.ServerInfo{
			Name: "tofu-ls",
		},
	}
	return result, nil
}

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

	formatted := tofufmt.Format(src, filename)
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

func pathFromURI(u uri.URI) (string, error) {
	// we only care about file scheme
	if u.Scheme() != "file" {
		return "", fmt.Errorf("Unsupported URI Scheme %q, only file:// is supported", u.Scheme())
	}

	path := u.Path()

	// TODO: windows support, they do strange stuff with path URIs

	return path, nil
}
