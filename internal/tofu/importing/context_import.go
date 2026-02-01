// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package importing

import (
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
)

// CommandLineImportTarget is a target that we need to import, that originated from the CLI command
// It represents a single resource that we need to import.
// The resource's ID and Address are fully known when executing the command (unlike when using the `import` block)
type CommandLineImportTarget struct {
	// Addr is the address for the resource instance that the new object should
	// be imported into.
	Addr addrs.AbsResourceInstance

	// ID is the string ID of the resource to import. This is resource-specific.
	ID string
}

// ImportTarget is a target that we need to import.
// It could either represent a single resource or multiple instances of the same resource, if for_each is used
// ImportTarget can be either a result of the import CLI command, or the import block
type ImportTarget struct {
	// Config is the original import block for this import. This might be null
	// if the import did not originate in config.
	// Config is mutually-exclusive with CommandLineImportTarget
	Config *configs.Import

	// CommandLineImportTarget is the ImportTarget information in the case of an import target origination for the
	// command line. CommandLineImportTarget is mutually-exclusive with Config
	*CommandLineImportTarget
}

// IsFromImportBlock checks whether the import target originates from an `import` block
// Currently, it should yield the opposite result of IsFromImportCommandLine, as those two are mutually-exclusive
func (i *ImportTarget) IsFromImportBlock() bool {
	return i.Config != nil
}

// IsFromImportCommandLine checks whether the import target originates from a `tofu import` command
// Currently, it should yield the opposite result of IsFromImportBlock, as those two are mutually-exclusive
func (i *ImportTarget) IsFromImportCommandLine() bool {
	return i.CommandLineImportTarget != nil
}

// StaticAddr returns the static address part of an import target
// For an ImportTarget originating from the command line, the address is already known
// However for an ImportTarget originating from an import block, the full address might not be known initially,
// and could only be evaluated down the line. Here, we create a static representation for the address.
// This is useful so that we could have information on the ImportTarget early on, such as the Module and Resource of it
func (i *ImportTarget) StaticAddr() addrs.ConfigResource {
	if i.IsFromImportCommandLine() {
		return i.CommandLineImportTarget.Addr.ConfigResource()
	}

	return i.Config.StaticTo
}

// ResolvedAddr returns a reference to the resolved address of an import target, if possible. If not possible, it
// returns nil.
// For an ImportTarget originating from the command line, the address is already known
// However for an ImportTarget originating from an import block, the full address might not be known initially,
// and could only be evaluated down the line.
func (i *ImportTarget) ResolvedAddr() *addrs.AbsResourceInstance {
	if i.IsFromImportCommandLine() {
		return &i.CommandLineImportTarget.Addr
	} else {
		return i.Config.ResolvedTo
	}
}
