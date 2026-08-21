// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planfile

import (
	"archive/zip"
	"bytes"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configload"
	"github.com/opentofu/opentofu/internal/depsfile"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states/statefile"
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

	// Schemas is the full set of provider schemas that were in memory at
	// the time the plan was created. If set, a trimmed-down subset
	// of these schemas is stored alongside the plan.
	// This is deliberately typed as a plain map, rather than as
	// *tofu.Schemas, so that this package doesn't need to import
	// internal/tofu (which would create an import cycle).
	Schemas map[addrs.Provider]providers.ProviderSchema

	// Config is the parsed configuration that the plan was created from.
	// It's used only to determine which parts of Schemas are actually
	// needed to render the plan (see Schemas above); it is not itself
	// written to the plan file; ConfigSnapshot is used for that.
	//
	// This may be left nil if Schemas is also nil.
	Config *configs.Config
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

	if err := writePlanEntry(zw, args.Plan); err != nil {
		return err
	}
	if err := writeStateFileEntry(zw, tfstateFilename, args.StateFile); err != nil {
		return err
	}
	if err := writeStateFileEntry(zw, tfstatePreviousFilename, args.PreviousRunStateFile); err != nil {
		return err
	}
	if err := writeConfigSnapshotEntry(zw, args.ConfigSnapshot); err != nil {
		return err
	}
	if err := writeDependencyLocksEntry(zw, args.DependencyLocks); err != nil {
		return err
	}
	if err := writeSchemasEntry(zw, args); err != nil {
		return err
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

func writePlanEntry(zw *zip.Writer, plan *plans.Plan) error {
	w, err := zw.CreateHeader(&zip.FileHeader{
		Name:     tfplanFilename,
		Method:   zip.Deflate,
		Modified: time.Now(),
	})
	if err != nil {
		return fmt.Errorf("failed to create tfplan file: %w", err)
	}
	if err := writePlan(plan, w); err != nil {
		return fmt.Errorf("failed to write plan: %w", err)
	}
	return nil
}

func writeStateFileEntry(zw *zip.Writer, name string, sf *statefile.File) error {
	w, err := zw.CreateHeader(&zip.FileHeader{
		Name:     name,
		Method:   zip.Deflate,
		Modified: time.Now(),
	})
	if err != nil {
		return fmt.Errorf("failed to create embedded %s file: %w", name, err)
	}
	if err := statefile.Write(sf, w, encryption.StateEncryptionDisabled()); err != nil {
		return fmt.Errorf("failed to write %s state snapshot: %w", name, err)
	}
	return nil
}

func writeConfigSnapshotEntry(zw *zip.Writer, snap *configload.Snapshot) error {
	if err := writeConfigSnapshot(snap, zw); err != nil {
		return fmt.Errorf("failed to write config snapshot: %w", err)
	}
	return nil
}

func writeDependencyLocksEntry(zw *zip.Writer, locks *depsfile.Locks) error {
	if locks == nil {
		return nil
	}

	src, diags := depsfile.SaveLocksToBytes(locks)
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
	if _, err := w.Write(src); err != nil {
		return fmt.Errorf("failed to write embedded dependency lock file: %w", err)
	}
	return nil
}

func writeSchemasEntry(zw *zip.Writer, args CreateArgs) error {
	trimmed := prepareSchemasForWrite(args.Plan, args.Config, args.Schemas)
	if len(trimmed) == 0 {
		log.Printf("[TRACE] planfile: no schemas embedded in plan file; tofu show will need to fetch schemas from providers instead")
		return nil
	}
	log.Printf("[TRACE] planfile: embedding trimmed schemas for %d provider(s) in plan file", len(trimmed))

	w, err := zw.CreateHeader(&zip.FileHeader{
		Name:     tfschemasFilename,
		Method:   zip.Deflate,
		Modified: time.Now(),
	})
	if err != nil {
		return fmt.Errorf("failed to create embedded schemas file: %w", err)
	}
	if err := writeSchemas(trimmed, w); err != nil {
		return fmt.Errorf("failed to write embedded schemas file: %w", err)
	}
	return nil
}
