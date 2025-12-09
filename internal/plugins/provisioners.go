package plugins

import (
	"context"
	"fmt"
	"sync"

	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/provisioners"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

type ProvisionerSchemas interface {
	HasProvisioner(typ string) bool
	ProvisionerSchema(typ string) (*configschema.Block, error)
}
type ProvisionerManager interface {
	ProvisionerSchemas

	ValidateProvisionerConfig(ctx context.Context, typ string, config cty.Value) tfdiags.Diagnostics
	ProvisionResource(ctx context.Context, typ string, config cty.Value, connection cty.Value, output provisioners.UIOutput) tfdiags.Diagnostics
}

type provisionerManager struct {
	factories map[string]provisioners.Factory

	instancesLock sync.Mutex
	instances     map[string]provisioners.Interface
}

func NewProvisionerManager(factories map[string]provisioners.Factory) ProvisionerManager {
	return &provisionerManager{
		factories: factories,
		instances: map[string]provisioners.Interface{},
	}
}

func (p *provisionerManager) HasProvisioner(typ string) bool {
	_, ok := p.factories[typ]
	return ok
}

func (p *provisionerManager) provisioner(typ string) (provisioners.Interface, error) {
	p.instancesLock.Lock()
	defer p.instancesLock.Unlock()

	instance, ok := p.instances[typ]
	if !ok {
		f, ok := p.factories[typ]
		if !ok {
			return nil, fmt.Errorf("unavailable provisioner %q", typ)
		}

		var err error
		instance, err = f()
		if err != nil {
			return nil, err
		}
		p.instances[typ] = instance
	}

	return instance, nil
}

// ProvisionerSchema uses a temporary instance of the provisioner with the
// given type name to obtain the schema for that provisioner's configuration.
//
// ProvisionerSchema memoizes results by provisioner type name, so it's fine
// to repeatedly call this method with the same name if various different
// parts of OpenTofu all need the same schema information.
func (p *provisionerManager) ProvisionerSchema(typ string) (*configschema.Block, error) {
	provisioner, err := p.provisioner(typ)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate provisioner %q to obtain schema: %w", typ, err)
	}
	defer provisioner.Close()

	resp := provisioner.GetSchema()
	if resp.Diagnostics.HasErrors() {
		return nil, fmt.Errorf("failed to retrieve schema from provisioner %q: %w", typ, resp.Diagnostics.Err())
	}

	return resp.Provisioner, nil
}

func (p *provisionerManager) ValidateProvisionerConfig(ctx context.Context, typ string, config cty.Value) tfdiags.Diagnostics {
	provisioner, err := p.provisioner(typ)
	if err != nil {
		return tfdiags.Diagnostics{}.Append(fmt.Errorf("failed to instantiate provisioner %q to validate config: %w", typ, err))
	}
	return provisioner.ValidateProvisionerConfig(provisioners.ValidateProvisionerConfigRequest{
		Config: config,
	}).Diagnostics
}

func (p *provisionerManager) ProvisionResource(ctx context.Context, typ string, config cty.Value, connection cty.Value, output provisioners.UIOutput) tfdiags.Diagnostics {
	provisioner, err := p.provisioner(typ)
	if err != nil {
		return tfdiags.Diagnostics{}.Append(fmt.Errorf("failed to instantiate provisioner %q to validate config: %w", typ, err))
	}
	return provisioner.ProvisionResource(provisioners.ProvisionResourceRequest{
		Config:     config,
		Connection: connection,
		UIOutput:   output,
	}).Diagnostics
}

func (p *provisionerManager) CloseProvisioners() error {
	p.instancesLock.Lock()
	defer p.instancesLock.Unlock()

	var diags tfdiags.Diagnostics
	for name, prov := range p.instances {
		err := prov.Close()
		if err != nil {
			diags = diags.Append(fmt.Errorf("provisioner.Close %s: %w", name, err))
		}
	}
	return diags.Err()
}
