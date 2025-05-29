package provisioners

import "github.com/opentofu/opentofu/internal/configs/configschema"

type Schemas interface {
	ProvisionerSchema(typ string) (*configschema.Block, error)
	ProvisionerSchemas() (map[string]*configschema.Block, error)
}
