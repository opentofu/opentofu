// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pg

import (
	"context"
	"crypto/md5"
	"database/sql"
	"fmt"
	"hash/fnv"

	"github.com/lib/pq"

	uuid "github.com/hashicorp/go-uuid"
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

// RemoteClient is a remote client that stores data in a Postgres database
type RemoteClient struct {
	Client     *sql.DB
	Name       string
	SchemaName string
	TableName  string
	IndexName  string

	info *statemgr.LockInfo
}

func (c *RemoteClient) Get(_ context.Context) (*remote.Payload, error) {
	query := fmt.Sprintf(`SELECT data FROM %s.%s WHERE name = $1`, pq.QuoteIdentifier(c.SchemaName), pq.QuoteIdentifier(c.TableName))
	row := c.Client.QueryRow(query, c.Name)
	var data []byte
	err := row.Scan(&data)
	switch {
	case err == sql.ErrNoRows:
		// No existing state returns empty.
		return nil, nil
	case err != nil:
		return nil, err
	default:
		md5 := md5.Sum(data)
		return &remote.Payload{
			Data: data,
			MD5:  md5[:],
		}, nil
	}
}

func (c *RemoteClient) Put(_ context.Context, data []byte) error {
	query := fmt.Sprintf(`INSERT INTO %s.%s (name, data) VALUES ($1, $2)
		ON CONFLICT (name) DO UPDATE
		SET data = $2 WHERE %s.name = $1`, pq.QuoteIdentifier(c.SchemaName), pq.QuoteIdentifier(c.TableName), pq.QuoteIdentifier(c.TableName))
	_, err := c.Client.Exec(query, c.Name, data)
	if err != nil {
		return err
	}
	return nil
}

func (c *RemoteClient) Delete(_ context.Context) error {
	query := fmt.Sprintf(`DELETE FROM %s.%s WHERE name = $1`, pq.QuoteIdentifier(c.SchemaName), pq.QuoteIdentifier(c.TableName))
	_, err := c.Client.Exec(query, c.Name)
	if err != nil {
		return err
	}
	return nil
}

func (c *RemoteClient) Lock(_ context.Context, info *statemgr.LockInfo) (string, error) {
	var err error
	var lockID string

	if info.ID == "" {
		lockID, err = uuid.GenerateUUID()
		if err != nil {
			return "", err
		}
		info.ID = lockID
	}

	// Local helper function so we can call it multiple places
	//
	lockUnlock := func(pgLockId string) error {
		query := `SELECT pg_advisory_unlock($1)`
		row := c.Client.QueryRow(query, pgLockId)
		var didUnlock []byte
		err := row.Scan(&didUnlock)
		if err != nil {
			return &statemgr.LockError{Info: info, Err: err}
		}
		return nil
	}

	creationLockID := c.composeCreationLockID()

	// Try to acquire locks for the existing row `id` and the creation lock.
	query := fmt.Sprintf(`SELECT %s.id, pg_try_advisory_lock(%s.id), pg_try_advisory_lock($1) FROM %s.%s WHERE %s.name = $2`,
		pq.QuoteIdentifier(c.TableName), pq.QuoteIdentifier(c.TableName), pq.QuoteIdentifier(c.SchemaName), pq.QuoteIdentifier(c.TableName), pq.QuoteIdentifier(c.TableName))

	row := c.Client.QueryRow(query, creationLockID, c.Name)
	var pgLockId, didLock, didLockForCreate []byte
	err = row.Scan(&pgLockId, &didLock, &didLockForCreate)
	switch {
	case err == sql.ErrNoRows:
		// No rows means we're creating the workspace. Take the creation lock.
		query = `SELECT pg_try_advisory_lock($1)`
		innerRow := c.Client.QueryRow(query, creationLockID)
		var innerDidLock []byte
		err := innerRow.Scan(&innerDidLock)
		if err != nil {
			return "", &statemgr.LockError{Info: info, Err: err}
		}
		if string(innerDidLock) == "false" {
			return "", &statemgr.LockError{Info: info, Err: fmt.Errorf("Already locked for workspace creation: %s", c.Name)}
		}
		info.Path = creationLockID
	case err != nil:
		return "", &statemgr.LockError{Info: info, Err: err}
	case string(didLock) == "false":
		// Existing workspace is already locked. Release the attempted creation lock.
		_ = lockUnlock(creationLockID)
		return "", &statemgr.LockError{Info: info, Err: fmt.Errorf("Workspace is already locked: %s", c.Name)}
	case string(didLockForCreate) == "false":
		// Someone has the creation lock already. Release the existing workspace because it might not be safe to touch.
		_ = lockUnlock(string(pgLockId))
		return "", &statemgr.LockError{Info: info, Err: fmt.Errorf("Cannot lock workspace; already locked for workspace creation: %s", c.Name)}
	default:
		// Existing workspace is now locked. Release the attempted creation lock.
		_ = lockUnlock(creationLockID)
		info.Path = string(pgLockId)
	}
	c.info = info

	return info.ID, nil
}

func (c *RemoteClient) Unlock(_ context.Context, id string) error {
	if c.info != nil && c.info.Path != "" {
		query := `SELECT pg_advisory_unlock($1)`
		row := c.Client.QueryRow(query, c.info.Path)
		var didUnlock []byte
		err := row.Scan(&didUnlock)
		if err != nil {
			return &statemgr.LockError{Info: c.info, Err: err}
		}
		c.info = nil
	}
	return nil
}

func (c *RemoteClient) composeCreationLockID() string {
	hash := fnv.New32()
	hash.Write([]byte(c.SchemaName + "\x00" + c.TableName))
	return fmt.Sprintf("%d", int64(hash.Sum32())*-1)
}
