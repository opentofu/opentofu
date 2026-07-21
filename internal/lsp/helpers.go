// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package lsp

import (
	"fmt"

	"go.lsp.dev/uri"
)

func pathFromURI(u uri.URI) (string, error) {
	if u.Scheme() != "file" {
		return "", fmt.Errorf("unsupported URI Scheme %q, only file:// is supported", u.Scheme())
	}

	path := u.Path()

	// TODO: windows support, they do strange stuff with path URIs

	return path, nil
}
