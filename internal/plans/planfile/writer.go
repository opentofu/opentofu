// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planfile

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"time"

	"github.com/terramate-io/opentofulib/internal/configs/configload"
	"github.com/terramate-io/opentofulib/internal/depsfile"
	"github.com/terramate-io/opentofulib/internal/encryption"
	"github.com/terramate-io/opentofulib/internal/plans"
	"github.com/terramate-io/opentofulib/internal/states/statefile"
)

type CreateArgs struct {
	// ConfigSnapshot is a snapshot of the configuration that the plan
	// was created from.
	ConfigSnapshot *configload.Snapshot

	// PreviousRunStateFile is a representation of the state snapshot we used
	// as the original input when creating this plan, containing the same
	// information as recorded at the end of the previous apply except for
	// upgrading managed resource instance data to the provider's latest
	// schema versions.
	PreviousRunStateFile *statefile.File

	// BaseStateFile is a representation of the state snapshot we used to
	// create the plan, which is the result of asking the providers to refresh
	// all previously-stored objects to match the current situation in the
	// remote system. (If this plan was created with refreshing disabled,
	// this should be the same as PreviousRunStateFile.)
	StateFile *statefile.File

	// Plan records the plan itself, which is the main artifact inside a
	// saved plan file.
	Plan *plans.Plan

	// DependencyLocks records the dependency lock information that we
	// checked prior to creating the plan, so we can make sure that all of the
	// same dependencies are still available when applying the plan.
	DependencyLocks *depsfile.Locks
}

// Create creates a new plan file with the given filename, overwriting any
// file that might already exist there.
//
// A plan file contains both a snapshot of the configuration and of the latest
// state file in addition to the plan itself, so that OpenTofu can detect
// if the world has changed since the plan was created and thus refuse to
// apply it.
func Create(filename string, args CreateArgs, enc encryption.PlanEncryption) error {
	buff := bytes.NewBuffer(make([]byte, 0))
	zw := zip.NewWriter(buff)

	// tfplan file
	{
		w, err := zw.CreateHeader(&zip.FileHeader{
			Name:     tfplanFilename,
			Method:   zip.Deflate,
			Modified: time.Now(),
		})
		if err != nil {
			return fmt.Errorf("failed to create tfplan file: %w", err)
		}
		err = writeTfplan(args.Plan, w)
		if err != nil {
			return fmt.Errorf("failed to write plan: %w", err)
		}
	}

	// tfstate file
	{
		w, err := zw.CreateHeader(&zip.FileHeader{
			Name:     tfstateFilename,
			Method:   zip.Deflate,
			Modified: time.Now(),
		})
		if err != nil {
			return fmt.Errorf("failed to create embedded tfstate file: %w", err)
		}
		err = statefile.Write(args.StateFile, w, encryption.StateEncryptionDisabled())
		if err != nil {
			return fmt.Errorf("failed to write state snapshot: %w", err)
		}
	}

	// tfstate-prev file
	{
		w, err := zw.CreateHeader(&zip.FileHeader{
			Name:     tfstatePreviousFilename,
			Method:   zip.Deflate,
			Modified: time.Now(),
		})
		if err != nil {
			return fmt.Errorf("failed to create embedded tfstate-prev file: %w", err)
		}
		err = statefile.Write(args.PreviousRunStateFile, w, encryption.StateEncryptionDisabled())
		if err != nil {
			return fmt.Errorf("failed to write previous state snapshot: %w", err)
		}
	}

	// tfconfig directory
	{
		err := writeConfigSnapshot(args.ConfigSnapshot, zw)
		if err != nil {
			return fmt.Errorf("failed to write config snapshot: %w", err)
		}
	}

	// .terraform.lock.hcl file, containing dependency lock information
	if args.DependencyLocks != nil { // (this was a later addition, so not all callers set it, but main callers should)
		src, diags := depsfile.SaveLocksToBytes(args.DependencyLocks)
		if diags.HasErrors() {
			return fmt.Errorf("failed to write embedded dependency lock file: %w", diags.Err())
		}

		w, err := zw.CreateHeader(&zip.FileHeader{
			Name:     dependencyLocksFilename,
			Method:   zip.Deflate,
			Modified: time.Now(),
		})
		if err != nil {
			return fmt.Errorf("failed to create embedded dependency lock file: %w", err)
		}
		_, err = w.Write(src)
		if err != nil {
			return fmt.Errorf("failed to write embedded dependency lock file: %w", err)
		}
	}

	// Finish zip file
	zw.Close()
	// Encrypt payload
	encrypted, err := enc.EncryptPlan(buff.Bytes())
	if err != nil {
		return err
	}
	return os.WriteFile(filename, encrypted, 0644)
}
