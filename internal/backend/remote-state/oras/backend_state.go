package oras

import "github.com/opentofu/opentofu/internal/states/statemgr"

func (b *Backend) Workspaces() ([]string, error) {
	panic("unimplemented")
}

func (b *Backend) DeleteWorkspace(name string, _ bool) error {
	panic("unimplemented")
}

func (b *Backend) StateMgr(name string) (statemgr.Full, error) {
	panic("unimplemented")
}
