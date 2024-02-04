package oras

import (
	"github.com/opentofu/opentofu/internal/states/remote"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

type RemoteClient struct{}

func (c *RemoteClient) Get() (*remote.Payload, error) {
	panic("unimplemented")
}

func (c *RemoteClient) Put(data []byte) error {
	panic("unimplemented")
}

func (c *RemoteClient) Delete() error {
	panic("unimplemented")
}

func (c *RemoteClient) Lock(info *statemgr.LockInfo) (string, error) {
	panic("unimplemented")
}

func (c *RemoteClient) Unlock(id string) error {
	panic("unimplemented")
}
