package plugins

import (
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/provisioners"
)

type Schemas interface {
	providers.Schemas
	provisioners.Schemas
}

func NewSchemas(provider providers.Schemas, provisioner provisioners.Schemas) Schemas {
	type providerSchemas providers.Schemas
	type provisionerSchemas provisioners.Schemas

	return &struct {
		providerSchemas
		provisionerSchemas
	}{
		providerSchemas:    provider,
		provisionerSchemas: provisioner,
	}
}
