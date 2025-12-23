package oras

import (
	"context"
	"fmt"
	"strings"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

func (b *Backend) StateMgr(ctx context.Context, workspace string) (statemgr.Full, error) {
	repo, err := b.getRepository()
	if err != nil {
		return nil, err
	}
	client := newRemoteClient(repo, workspace)
	client.retryConfig = b.retryCfg
	client.versioningEnabled = b.versioningEnabled
	client.versioningMaxVersions = b.versioningMaxVersions
	client.stateCompression = b.compression
	client.lockTTL = b.lockTTL
	return remote.NewState(client, b.encryption), nil
}

func (b *Backend) Workspaces(ctx context.Context) ([]string, error) {
	repo, err := b.getRepository()
	if err != nil {
		return nil, err
	}
	wss, err := listWorkspacesFromTags(ctx, repo)
	if err != nil {
		if isNotFound(err) {
			return []string{backend.DefaultStateName}, nil
		}
		return nil, err
	}
	if len(wss) == 0 {
		return []string{backend.DefaultStateName}, nil
	}
	return wss, nil
}

func (b *Backend) DeleteWorkspace(ctx context.Context, name string, _ bool) error {
	if name == backend.DefaultStateName || name == "" {
		return fmt.Errorf("can't delete default state")
	}

	repo, err := b.getRepository()
	if err != nil {
		return err
	}

	wsTag := workspaceTagFor(name)
	stateRef := stateTagPrefix + wsTag
	lockRef := lockTagPrefix + wsTag
	stateVersionPrefix := stateRef + stateVersionTagSeparator

	if desc, err := repo.inner.Resolve(ctx, stateRef); err == nil {
		_ = repo.inner.Delete(ctx, desc)
	}
	_ = repo.inner.Tags(ctx, "", func(page []string) error {
		for _, tag := range page {
			if !strings.HasPrefix(tag, stateVersionPrefix) {
				continue
			}
			if desc, err := repo.inner.Resolve(ctx, tag); err == nil {
				_ = repo.inner.Delete(ctx, desc)
			}
		}
		return nil
	})
	if desc, err := repo.inner.Resolve(ctx, lockRef); err == nil {
		_ = repo.inner.Delete(ctx, desc)
	}
	return nil
}
