// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package libraries

import (
	"github.com/hashicorp/go-version"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/getproviders"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ProviderRequirement describes a dependency between a library and a
// provider.
type ProviderRequirement struct {
	Versions getproviders.VersionConstraints

	DeclRange, AddrRange, VersionsRange tfdiags.SourceRange
}

// LibraryRequirement describes a dependency from one library to another.
type LibraryRequirement struct {
	SourceAddr addrs.ModuleSource
	Versions   *version.Constraint

	DeclRange, SourceAddrRange, VersionsRange tfdiags.SourceRange
}
