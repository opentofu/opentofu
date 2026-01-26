// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plugins

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/provisioners"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

type ProvisionerFactories map[string]provisioners.Factory

func (p ProvisionerFactories) HasProvisioner(typ string) bool {
	_, ok := p[typ]
	return ok
}

func (p ProvisionerFactories) NewInstance(typ string) (provisioners.Interface, error) {
	f, ok := p[typ]
	if !ok {
		return nil, fmt.Errorf("unavailable provisioner %q", typ)
	}

	return f()
}

type ProvisionerManager interface {
	HasProvisioner(typ string) bool
	ProvisionerSchema(typ string) (*configschema.Block, error)
	ValidateProvisionerConfig(ctx context.Context, typ string, config cty.Value) tfdiags.Diagnostics
	ProvisionResource(ctx context.Context, typ string, config cty.Value, connection cty.Value, output provisioners.UIOutput) tfdiags.Diagnostics
	Close() error
	Stop() error
}

type provisionerManager struct {
	*library

	instancesLock sync.Mutex
	instances     map[string]provisioners.Interface
}

func (l *library) NewProvisionerManager() ProvisionerManager {
	return &provisionerManager{
		library:   l,
		instances: map[string]provisioners.Interface{},
	}
}

func (p *provisionerManager) HasProvisioner(typ string) bool {
	return p.provisionerFactories.HasProvisioner(typ)
}

func (p *provisionerManager) provisioner(typ string) (provisioners.Interface, error) {
	p.instancesLock.Lock()
	defer p.instancesLock.Unlock()

	instance, ok := p.instances[typ]
	if !ok {
		var err error
		instance, err = p.provisionerFactories.NewInstance(typ)
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
	// Coarse lock only for ensuring that a valid entry exists
	p.provisionerSchemasLock.Lock()
	entry, ok := p.provisionerSchemas[typ]
	if !ok {
		entry = sync.OnceValues(func() (*configschema.Block, error) {
			log.Printf("[TRACE] Initializing provisioner %q to read its schema", typ)
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
		})
		p.provisionerSchemas[typ] = entry
	}
	// This lock is only for access to the map. We don't need to hold the lock when calling
	// "entry" because [sync.OnceValues] handles synchronization itself.
	// We don't defer unlock as the majority of the work of this function happens in calling "entry"
	// and we want to release as soon as possible for multiple concurrent callers of different provisioners
	p.provisionerSchemasLock.Unlock()

	return entry()
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

func (p *provisionerManager) Close() error {
	p.instancesLock.Lock()
	defer p.instancesLock.Unlock()

	var diags tfdiags.Diagnostics
	for name, prov := range p.instances {
		err := prov.Close()
		if err != nil {
			diags = diags.Append(fmt.Errorf("provisioner.Close %s: %w", name, err))
		}
	}
	clear(p.instances)
	return diags.Err()
}

func (p *provisionerManager) Stop() error {
	p.instancesLock.Lock()
	defer p.instancesLock.Unlock()

	var diags tfdiags.Diagnostics
	for name, prov := range p.instances {
		err := prov.Stop()
		if err != nil {
			diags = diags.Append(fmt.Errorf("provisioner.Stop %s: %w", name, err))
		}
	}
	clear(p.instances)
	return diags.Err()
}
