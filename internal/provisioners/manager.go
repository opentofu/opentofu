package provisioners

import (
	"errors"
	"fmt"
	"log"

	"github.com/opentofu/opentofu/internal/configs/configschema"
)

type Manager interface {
	Schemas
	NewProvisionerInstance(typ string) (Interface, error)
}

type manager struct {
	schemas   map[string]*configschema.Block
	factories map[string]Factory
}

func NewManager(factories map[string]Factory) (Manager, error) {
	m := &manager{
		schemas:   map[string]*configschema.Block{},
		factories: factories,
	}

	var errs []error
	// TODO parallelize
	for typ := range factories {

		// Initialize
		log.Printf("[TRACE] provisioners.Manager: Initializing provisioner %q to read its schema", typ)
		instance, err := m.NewProvisionerInstance(typ)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to initialize provisioner %q: %w", typ, err))
			continue
		}

		// Pre-load the schemas
		resp := instance.GetSchema()
		m.schemas[typ] = resp.Provisioner

		err = instance.Close()
		if err != nil {
			errs = append(errs, err)
		}

		// Validate
		if resp.Diagnostics.HasErrors() {
			errs = append(errs, fmt.Errorf("failed to retrieve schema from provisioner %q: %w", typ, resp.Diagnostics.Err()))
		}
	}

	return m, errors.Join(errs...)
}

func (m *manager) NewProvisionerInstance(typ string) (Interface, error) {
	f, ok := m.factories[typ]
	if !ok {
		return nil, fmt.Errorf("unavailable provisioner %q", typ)
	}

	return f()
}

func (m *manager) ProvisionerSchemas() map[string]*configschema.Block {
	// TODO copy
	return m.schemas
}

func (m *manager) HasProvisioner(typ string) bool {
	_, ok := m.factories[typ]
	return ok
}

func (m *manager) ProvisionerSchema(typ string) (*configschema.Block, error) {
	schema, ok := m.schemas[typ]
	if !ok {
		return nil, fmt.Errorf("unavailable provisioner %q", typ)
	}

	return schema, nil
}
