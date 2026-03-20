// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/addrs"
)

type StateStoreConfig struct {
	Type      string
	Provider  addrs.Provider
	Config    hcl.Body
	DeclRange hcl.Range

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

	return s, diags
}

func (s *StateStoreConfig) ProviderConfigAddr() addrs.LocalProviderConfig {
	// From: addrs.Resource.ImpliedProvider()
	typeName := s.Type
	if under := strings.Index(typeName, "_"); under != -1 {
		typeName = typeName[:under]
	}

	return addrs.LocalProviderConfig{
		LocalName: typeName,
	}
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
