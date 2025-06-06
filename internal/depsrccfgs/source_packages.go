// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package depsrccfgs

import (
	"github.com/hashicorp/hcl/v2"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

type SourcePackageRule struct {
	MatchPattern SourceAddrPattern
}

func decodeSourcePackageRuleBlock(block *hcl.Block) (*SourcePackageRule, tfdiags.Diagnostics) {
	panic("unimplemented")
}
