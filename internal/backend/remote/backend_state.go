// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package remote

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	tfe "github.com/hashicorp/go-tfe"

	"github.com/opentofu/opentofu/internal/command/jsonstate"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statefile"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

type remoteClient struct {
	client         *tfe.Client
	lockInfo       *statemgr.LockInfo
	organization   string
	runID          string
	stateUploadErr bool
	workspace      *tfe.Workspace
	forcePush      bool
	encryption     encryption.StateEncryption
}

// Get the remote state.
func (r *remoteClient) Get(ctx context.Context) (*remote.Payload, error) {
	sv, err := r.client.StateVersions.ReadCurrent(ctx, r.workspace.ID)
	if err != nil {
		if err == tfe.ErrResourceNotFound {
			// If no state exists, then return nil.
			return nil, nil
		}
		return nil, fmt.Errorf("Error retrieving state: %w", err)
	}

	state, err := r.client.StateVersions.Download(ctx, sv.DownloadURL)
	if err != nil {
		return nil, fmt.Errorf("Error downloading state: %w", err)
	}

	// If the state is empty, then return nil.
	if len(state) == 0 {
		return nil, nil
	}

	// Get the MD5 checksum of the state.
	sum := md5.Sum(state)

	return &remote.Payload{
		Data: state,
		MD5:  sum[:],
	}, nil
}

func (r *remoteClient) uploadStateFallback(ctx context.Context, stateFile *statefile.File, state []byte, jsonStateOutputs []byte) error {
	options := tfe.StateVersionCreateOptions{
		Lineage:          tfe.String(stateFile.Lineage),
		Serial:           tfe.Int64(int64(stateFile.Serial)),
		MD5:              tfe.String(fmt.Sprintf("%x", md5.Sum(state))),
		Force:            tfe.Bool(r.forcePush),
		State:            tfe.String(base64.StdEncoding.EncodeToString(state)),
		JSONStateOutputs: tfe.String(base64.StdEncoding.EncodeToString(jsonStateOutputs)),
	}

	// If we have a run ID, make sure to add it to the options
	// so the state will be properly associated with the run.
	if r.runID != "" {
		options.Run = &tfe.Run{ID: r.runID}
	}

	// Create the new state.
	_, err := r.client.StateVersions.Create(ctx, r.workspace.ID, options)
	if err != nil {
		r.stateUploadErr = true
		return fmt.Errorf("error uploading state in compatibility mode: %w", err)
	}
	return err
}

// Put the remote state.
func (r *remoteClient) Put(ctx context.Context, state []byte) error {
	// Read the raw state into a OpenTofu state.
	stateFile, err := statefile.Read(bytes.NewReader(state), r.encryption)
	if err != nil {
		return fmt.Errorf("error reading state: %w", err)
	}

	ov, err := jsonstate.MarshalOutputs(stateFile.State.RootModule().OutputValues)
	if err != nil {
		return fmt.Errorf("error reading output values: %w", err)
	}
	o, err := json.Marshal(ov)
	if err != nil {
		return fmt.Errorf("error converting output values to json: %w", err)
	}

	options := tfe.StateVersionUploadOptions{
		StateVersionCreateOptions: tfe.StateVersionCreateOptions{
			Lineage:          tfe.String(stateFile.Lineage),
			Serial:           tfe.Int64(int64(stateFile.Serial)),
			MD5:              tfe.String(fmt.Sprintf("%x", md5.Sum(state))),
			Force:            tfe.Bool(r.forcePush),
			JSONStateOutputs: tfe.String(base64.StdEncoding.EncodeToString(o)),
		},
		RawState: state,
	}

	// If we have a run ID, make sure to add it to the options
	// so the state will be properly associated with the run.
	if r.runID != "" {
		options.Run = &tfe.Run{ID: r.runID}
	}

	// Create the new state.
	// Create the new state.
	_, err = r.client.StateVersions.Upload(ctx, r.workspace.ID, options)
	if errors.Is(err, tfe.ErrStateVersionUploadNotSupported) {
		// Create the new state with content included in the request (Terraform Enterprise v202306-1 and below)
		log.Println("[INFO] Detected that state version upload is not supported. Retrying using compatibility state upload.")
		return r.uploadStateFallback(ctx, stateFile, state, o)
	}
	if err != nil {
		r.stateUploadErr = true
		return fmt.Errorf("error uploading state: %w", err)
	}

	return nil
}

// Delete the remote state.
func (r *remoteClient) Delete(ctx context.Context) error {
	err := r.client.Workspaces.Delete(ctx, r.organization, r.workspace.Name)
	if err != nil && err != tfe.ErrResourceNotFound {
		return fmt.Errorf("error deleting workspace %s: %w", r.workspace.Name, err)
	}

	return nil
}

// EnableForcePush to allow the remote client to overwrite state
// by implementing remote.ClientForcePusher
func (r *remoteClient) EnableForcePush() {
	r.forcePush = true
}

// Lock the remote state.
func (r *remoteClient) Lock(ctx context.Context, info *statemgr.LockInfo) (string, error) {
	lockErr := &statemgr.LockError{Info: r.lockInfo}

	// Lock the workspace.
	_, err := r.client.Workspaces.Lock(ctx, r.workspace.ID, tfe.WorkspaceLockOptions{
		Reason: tfe.String("Locked by OpenTofu"),
	})
	if err != nil {
		if err == tfe.ErrWorkspaceLocked {
			lockErr.Info = info
			err = fmt.Errorf("%s (lock ID: \"%s/%s\")", err, r.organization, r.workspace.Name)
		}
		lockErr.Err = err
		return "", lockErr
	}

	r.lockInfo = info

	return r.lockInfo.ID, nil
}

// Unlock the remote state.
func (r *remoteClient) Unlock(ctx context.Context, id string) error {
	// We first check if there was an error while uploading the latest
	// state. If so, we will not unlock the workspace to prevent any
	// changes from being applied until the correct state is uploaded.
	if r.stateUploadErr {
		return nil
	}

	lockErr := &statemgr.LockError{Info: r.lockInfo}

	// With lock info this should be treated as a normal unlock.
	if r.lockInfo != nil {
		// Verify the expected lock ID.
		if r.lockInfo.ID != id {
			lockErr.Err = fmt.Errorf("lock ID does not match existing lock")
			return lockErr
		}

		// Unlock the workspace.
		_, err := r.client.Workspaces.Unlock(ctx, r.workspace.ID)
		if err != nil {
			lockErr.Err = err
			return lockErr
		}

		return nil
	}

	// Verify the optional force-unlock lock ID.
	if r.organization+"/"+r.workspace.Name != id {
		lockErr.Err = fmt.Errorf(
			"lock ID %q does not match existing lock ID \"%s/%s\"",
			id,
			r.organization,
			r.workspace.Name,
		)
		return lockErr
	}

	// Force unlock the workspace.
	_, err := r.client.Workspaces.ForceUnlock(ctx, r.workspace.ID)
	if err != nil {
		lockErr.Err = err
		return lockErr
	}

	return nil
}
