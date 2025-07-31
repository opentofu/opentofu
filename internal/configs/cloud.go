// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"github.com/hashicorp/hcl/v2"
)

// Cloud represents a "cloud" block inside a "terraform" block in a module
// or file.
type CloudConfig struct {
	Config hcl.Body
	eval   *StaticEvaluator

	DeclRange hcl.Range
}

func decodeCloudBlock(block *hcl.Block) (*CloudConfig, hcl.Diagnostics) {
	return &CloudConfig{
		Config:    block.Body,
		DeclRange: block.DefRange,
	}, nil
}

func (c *CloudConfig) ToBackendConfig() Backend {
	return Backend{
		Type:   "cloud",
		Config: c.Config,
		Eval:   c.eval,
	}
}
