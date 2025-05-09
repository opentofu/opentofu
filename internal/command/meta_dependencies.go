// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"fmt"
	"log"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/depsfile"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// dependencyLockFilename is the filename of the dependency lock file.
//
// This file should live in the same directory as the .tf files for the
// root module of the configuration, alongside the .terraform directory
// as long as that directory's path isn't overridden by the TF_DATA_DIR
// environment variable.
//
// We always expect to find this file in the current working directory
// because that should also be the root module directory.
//
// Some commands have legacy command line arguments that make the root module
// directory something other than the root module directory; when using those,
// the lock file will be written in the "wrong" place (the current working
// directory instead of the root module directory) but we do that intentionally
// to match where the ".terraform" directory would also be written in that
// case. Eventually we will phase out those legacy arguments in favor of the
// global -chdir=... option, which _does_ preserve the intended invariant
// that the root module directory is always the current working directory.
const dependencyLockFilename = ".terraform.lock.hcl"

// lockedDependencies reads the dependency lock information from the lock file
// in the current working directory.
//
// If the lock file doesn't exist at the time of the call, lockedDependencies
// indicates success and returns an empty Locks object. If the file does
// exist then the result is either a representation of the contents of that
// file at the instant of the call or error diagnostics explaining some way
// in which the lock file is invalid.
//
// The result is a snapshot of the locked dependencies at the time of the call
// and does not update as a result of calling replaceLockedDependencies
// or any other modification method.
func (m *Meta) lockedDependencies() (*depsfile.Locks, tfdiags.Diagnostics) {
	// We check that the file exists first, because the underlying HCL
	// parser doesn't distinguish that error from other error types
	// in a machine-readable way but we want to treat that as a success
	// with no locks. There is in theory a race condition here in that
	// the file could be created or removed in the meantime, but we're not
	// promising to support two concurrent dependency installation processes.
	_, err := os.Stat(dependencyLockFilename)
	if os.IsNotExist(err) {
		return m.annotateDependencyLocksWithOverrides(depsfile.NewLocks()), nil
	}

	ret, diags := depsfile.LoadLocksFromFile(dependencyLockFilename)

	// If this is the first run after switching from OpenTofu's predecessor,
	// the lock file might contain some entries from the predecessor's registry
	// which we can translate into similar entries for OpenTofu's registry.
	changed := ret.UpgradeFromPredecessorProject()
	if len(changed) != 0 {
		oldAddrs := slices.Collect(maps.Keys(changed))
		slices.SortFunc(oldAddrs, func(a, b addrs.Provider) int {
			if a.LessThan(b) {
				return -1
			} else if b.LessThan(a) {
				return 1
			} else {
				return 0
			}
		})
		var buf strings.Builder // strings.Builder writes cannot fail
		_, _ = buf.WriteString("OpenTofu automatically rewrote some entries in your dependency lock file:\n")
		for _, oldAddr := range oldAddrs {
			newAddr := changed[oldAddr]
			// We intentionally use String instead of ForDisplay here because
			// this message won't make much sense without using fully-qualified
			// addresses with explicit registry hostnames.
			_, _ = fmt.Fprintf(&buf, "  - %s => %s\n", oldAddr.String(), newAddr.String())
		}
		_, _ = buf.WriteString("\nThe version selections were preserved, but the hashes were not because the OpenTofu project's provider releases are not byte-for-byte identical.")
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Warning,
			"Dependency lock file entries automatically updated",
			buf.String(),
		))
	}

	return m.annotateDependencyLocksWithOverrides(ret), diags
}

// replaceLockedDependencies creates or overwrites the lock file in the
// current working directory to contain the information recorded in the given
// locks object.
func (m *Meta) replaceLockedDependencies(ctx context.Context, new *depsfile.Locks) tfdiags.Diagnostics {
	return depsfile.SaveLocksToFile(ctx, new, dependencyLockFilename)
}

// annotateDependencyLocksWithOverrides modifies the given Locks object in-place
// to track as overridden any provider address that's subject to testing
// overrides, development overrides, or "unmanaged provider" status.
//
// This is just an implementation detail of the lockedDependencies method,
// not intended for use anywhere else.
func (m *Meta) annotateDependencyLocksWithOverrides(ret *depsfile.Locks) *depsfile.Locks {
	if ret == nil {
		return ret
	}

	for addr := range m.ProviderDevOverrides {
		log.Printf("[DEBUG] Provider %s is overridden by dev_overrides", addr)
		ret.SetProviderOverridden(addr)
	}
	for addr := range m.UnmanagedProviders {
		log.Printf("[DEBUG] Provider %s is overridden as an \"unmanaged provider\"", addr)
		ret.SetProviderOverridden(addr)
	}
	if m.testingOverrides != nil {
		for addr := range m.testingOverrides.Providers {
			log.Printf("[DEBUG] Provider %s is overridden in Meta.testingOverrides", addr)
			ret.SetProviderOverridden(addr)
		}
	}

	return ret
}
