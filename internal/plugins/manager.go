package plugins

import (
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/provisioners"
)

type Manager interface {
	providers.Manager
	provisioners.Manager
}

func NewManager(provider providers.Manager, provisioner provisioners.Manager) Manager {
	type providerManager providers.Manager
	type provisionerManager provisioners.Manager

	return &struct {
		providerManager
		provisionerManager
	}{
		provider,
		provisioner,
	}
}
