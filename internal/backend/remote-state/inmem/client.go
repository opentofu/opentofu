// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package inmem

import (
	"context"
	"crypto/md5"

	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

// RemoteClient is a remote client that stores data in memory for testing.
type RemoteClient struct {
	Data []byte
	MD5  []byte
	Name string
}

func (c *RemoteClient) Get(context.Context) (*remote.Payload, error) {
	if c.Data == nil {
		return nil, nil
	}

	return &remote.Payload{
		Data: c.Data,
		MD5:  c.MD5,
	}, nil
}

func (c *RemoteClient) Put(_ context.Context, data []byte) error {
	md5 := md5.Sum(data)

	c.Data = data
	c.MD5 = md5[:]
	return nil
}

func (c *RemoteClient) Delete(context.Context) error {
	c.Data = nil
	c.MD5 = nil
	return nil
}

func (c *RemoteClient) Lock(info *statemgr.LockInfo) (string, error) {
	return locks.lock(c.Name, info)
}
func (c *RemoteClient) Unlock(id string) error {
	return locks.unlock(c.Name, id)
}
