// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package r2

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	"github.com/opentofu/opentofu/internal/states/statefile"
)

// Workspaces returns a list of workspace names
func (b *Backend) Workspaces(ctx context.Context) ([]string, error) {
	
	// List all objects with the workspace prefix
	prefix := ""
	if b.workspaceKeyPrefix != "" {
		prefix = b.workspaceKeyPrefix + "/"
	}
	
	url := fmt.Sprintf("%s/%s?list-type=2&prefix=%s&max-keys=1000", b.getR2Endpoint(), b.bucketName, prefix)
	
	workspaces := []string{backend.DefaultStateName}
	workspaceSet := make(map[string]bool)
	
	for {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}
		
		// Add authentication
		req.Header.Set("Authorization", "Bearer "+b.apiToken)
		
		resp, err := b.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != 200 {
			// If access denied, return default workspace only (backward compatibility)
			if resp.StatusCode == 403 {
				return workspaces, nil
			}
			return nil, fmt.Errorf("failed to list workspaces: status %d", resp.StatusCode)
		}
		
		// Parse XML response
		var result listBucketResult
		if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to parse list response: %w", err)
		}
		
		for _, obj := range result.Contents {
			ws := b.keyEnv(obj.Key)
			if ws != "" && !workspaceSet[ws] {
				workspaceSet[ws] = true
				workspaces = append(workspaces, ws)
			}
		}
		
		if !result.IsTruncated {
			break
		}
		
		// Continue pagination
		url = fmt.Sprintf("%s/%s?list-type=2&prefix=%s&max-keys=1000&continuation-token=%s", 
			b.getR2Endpoint(), b.bucketName, prefix, result.NextContinuationToken)
	}
	
	sort.Strings(workspaces[1:])
	return workspaces, nil
}

// DeleteWorkspace deletes the specified workspace
func (b *Backend) DeleteWorkspace(ctx context.Context, name string, force bool) error {
	if name == backend.DefaultStateName || name == "" {
		return fmt.Errorf("cannot delete default workspace")
	}
	client, err := b.remoteClient(name)
	if err != nil {
		return err
	}
	
	return client.Delete(ctx)
}

// StateMgr returns a state manager for the specified workspace
func (b *Backend) StateMgr(ctx context.Context, name string) (statemgr.Full, error) {
	client, err := b.remoteClient(name)
	if err != nil {
		return nil, err
	}
	
	stateMgr := remote.NewState(client, b.encryption)
	
	// Initialize the state manager
	if err := stateMgr.RefreshState(ctx); err != nil {
		// If the state doesn't exist, create an empty one
		if err == statefile.ErrNoState {
			if err := stateMgr.WriteState(states.NewState()); err != nil {
				return nil, fmt.Errorf("failed to initialize state: %w", err)
			}
			if err := stateMgr.PersistState(ctx, nil); err != nil {
				return nil, fmt.Errorf("failed to persist initial state: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to refresh state: %w", err)
		}
	}
	
	return stateMgr, nil
}

// remoteClient returns a RemoteClient for the given workspace
func (b *Backend) remoteClient(name string) (*RemoteClient, error) {
	if name == "" {
		return nil, fmt.Errorf("workspace name cannot be empty")
	}
	
	key := b.key
	if name != backend.DefaultStateName {
		// Don't use path.Join as it cleans the path and removes trailing colons
		key = fmt.Sprintf("%s%s/%s", b.workspaceKeyPrefix, name, b.key)
	}
	
	return &RemoteClient{
		backend:    b,
		bucketName: b.bucketName,
		key:        key,
	}, nil
}

// keyEnv extracts the workspace name from a key
func (b *Backend) keyEnv(key string) string {
	prefix := b.workspaceKeyPrefix

	if prefix == "" {
		parts := strings.SplitN(key, "/", 2)
		if len(parts) > 1 && parts[1] == b.key {
			return parts[0]
		}
		return ""
	}

	// Add a slash to treat this as a directory
	prefix += "/"

	parts := strings.SplitAfterN(key, prefix, 2)
	if len(parts) < 2 {
		return ""
	}

	// Shouldn't happen since we listed by prefix
	if parts[0] != prefix {
		return ""
	}

	// Get the workspace name from the path
	end := strings.Index(parts[1], "/")
	if end == -1 {
		return ""
	}

	ws := parts[1][:end]

	// Check that the rest of the key matches our state file name
	remainder := parts[1][end+1:]
	if remainder != b.key {
		return ""
	}

	return ws
}

// listBucketResult represents the XML response from listing bucket contents
type listBucketResult struct {
	XMLName                xml.Name `xml:"ListBucketResult"`
	IsTruncated            bool     `xml:"IsTruncated"`
	Contents               []s3Object `xml:"Contents"`
	Name                   string   `xml:"Name"`
	Prefix                 string   `xml:"Prefix"`
	MaxKeys                int      `xml:"MaxKeys"`
	NextContinuationToken  string   `xml:"NextContinuationToken"`
}

// s3Object represents an object in the S3 bucket
type s3Object struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
	StorageClass string `xml:"StorageClass"`
}

// StateSnapshotMeta returns metadata about the state snapshot
func (b *Backend) StateSnapshotMeta() statemgr.SnapshotMeta {
	return statemgr.SnapshotMeta{}
}
