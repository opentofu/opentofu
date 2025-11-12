// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"fmt"
)

// SuffixUIInput is an implementation of UIInput that suffixes the query
// with a string, allowing to add specific hints about the configuration
// of the variable.
type SuffixUIInput struct {
	QuerySuffix string
	UIInput     UIInput
}

func (i *SuffixUIInput) Input(ctx context.Context, opts *InputOpts) (string, error) {
	opts.Query = fmt.Sprintf("%s%s", opts.Query, i.QuerySuffix)
	return i.UIInput.Input(ctx, opts)
}

// NewEphemeralSuffixUIInput creates a new SuffixUIInput that adds " (ephemeral)" hint
// to the query string.
func NewEphemeralSuffixUIInput(input UIInput) UIInput {
	return &SuffixUIInput{
		QuerySuffix: " (ephemeral)",
		UIInput:     input,
	}
}
