// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pg

import (
	"context"
	"fmt"

	"github.com/lib/pq"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

func (b *Backend) Workspaces(context.Context) ([]string, error) {
	query := fmt.Sprintf(`SELECT name FROM %s.%s WHERE name != 'default' ORDER BY name`, pq.QuoteIdentifier(b.schemaName), pq.QuoteIdentifier(b.tableName))
	rows, err := b.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []string{
		backend.DefaultStateName,
	}

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		result = append(result, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func (b *Backend) DeleteWorkspace(_ context.Context, name string, _ bool) error {
	if name == backend.DefaultStateName || name == "" {
		return fmt.Errorf("can't delete default state")
	}

	query := fmt.Sprintf(`DELETE FROM %s.%s WHERE name = $1`, pq.QuoteIdentifier(b.schemaName), pq.QuoteIdentifier(b.tableName))
	_, err := b.db.Exec(query, name)
	if err != nil {
		return err
	}

	return nil
}

func (b *Backend) StateMgr(ctx context.Context, name string) (statemgr.Full, error) {
	// Build the state client
	var stateMgr statemgr.Full = remote.NewState(
		&RemoteClient{
			Client:     b.db,
			Name:       name,
			SchemaName: b.schemaName,
			TableName:  b.tableName,
			IndexName:  b.indexName,
		},
		b.encryption,
	)

	// Check to see if this state already exists.
	// If the state doesn't exist, we have to assume this
	// is a normal create operation, and take the lock at that point.
	existing, err := b.Workspaces(ctx)
	if err != nil {
		return nil, err
	}

	exists := false
	for _, s := range existing {
		if s == name {
			exists = true
			break
		}
	}

	// Grab a lock, we use this to write an empty state if one doesn't
	// exist already. We have to write an empty state as a sentinel value
	// so Workspaces() knows it exists.
	if !exists {
		lockInfo := statemgr.NewLockInfo()
		lockInfo.Operation = "init"
		lockId, err := stateMgr.Lock(lockInfo)
		if err != nil {
			return nil, fmt.Errorf("failed to lock state in Postgres: %w", err)
		}

		// Local helper function so we can call it multiple places
		lockUnlock := func(parent error) error {
			if err := stateMgr.Unlock(lockId); err != nil {
				return fmt.Errorf("error unlocking Postgres state: %w", err)
			}
			return parent
		}

		if v := stateMgr.State(); v == nil {
			if err := stateMgr.WriteState(states.NewState()); err != nil {
				err = lockUnlock(err)
				return nil, err
			}
			if err := stateMgr.PersistState(nil); err != nil {
				err = lockUnlock(err)
				return nil, err
			}
		}

		// Unlock, the state should now be initialized
		if err := lockUnlock(nil); err != nil {
			return nil, err
		}
	}

	return stateMgr, nil
}
