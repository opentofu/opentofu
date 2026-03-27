// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/addrs"
)

type StateStoreConfig struct {
	Type          string
	LocalProvider addrs.LocalProviderConfig
	Provider      addrs.Provider
	Config        hcl.Body
	DeclRange     hcl.Range

	eval *StaticEvaluator
}

func decodeStateStoreBlock(block *hcl.Block) (*StateStoreConfig, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	s := &StateStoreConfig{
		Type:      block.Labels[0],
		Config:    block.Body,
		DeclRange: block.DefRange,
	}

	if !hclsyntax.ValidIdentifier(s.Type) {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid state_store type name",
			Detail:   badIdentifierDetail,
			Subject:  &block.LabelRanges[0],
		})
	}

	content, _, moreDiags := block.Body.PartialContent(stateStoreBlockSchema)
	diags = diags.Extend(moreDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	var prevProvider *hcl.Range
	for _, block := range content.Blocks {
		if block.Type == "provider" {
			s.LocalProvider = addrs.LocalProviderConfig{LocalName: block.Labels[0]}
			if prevProvider != nil {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate provider block",
					Detail:   fmt.Sprintf("state_store already has a provider block at %s.", prevProvider.String()),
					Subject:  &block.DefRange,
				})
				continue
			}
			prevProvider = block.DefRange.Ptr()
		}
	}

	if prevProvider == nil {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Missing provider block",
			Detail:   "state_store requires a provider block in order to determine what provider should be used",
			Subject:  block.DefRange.Ptr(),
		})
	}

	return s, diags
}

func (s *StateStoreConfig) ToBackendConfig() Backend {
	return Backend{
		Type:               "state_store",
		Config:             s.Config,
		Eval:               s.eval,
		StateStoreType:     s.Type,
		StateStoreProvider: s.Provider,
	}
}

var stateStoreBlockSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{
			Type:       "provider",
			LabelNames: []string{"name"},
		},
	},
}
