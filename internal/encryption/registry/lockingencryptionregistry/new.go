package lockingencryptionregistry

import (
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/method"
	"github.com/opentofu/opentofu/internal/encryption/registry"
	"sync"
)

// New returns a new encryption registry that locks for parallel access.
func New() registry.Registry {
	return &lockingRegistry{
		lock:      &sync.RWMutex{},
		providers: map[keyprovider.ID]keyprovider.Factory{},
		methods:   map[method.ID]method.Factory{},
	}
}

type lockingRegistry struct {
	lock      *sync.RWMutex
	providers map[keyprovider.ID]keyprovider.Factory
	methods   map[method.ID]method.Factory
}

func (l *lockingRegistry) RegisterKeyProvider(keyProvider keyprovider.Factory) error {
	l.lock.Lock()
	defer l.lock.Unlock()

	id := keyProvider.ID()
	if err := id.Validate(); err != nil {
		return &registry.InvalidKeyProvider{KeyProvider: keyProvider, Cause: err}
	}
	if _, ok := l.providers[id]; ok {
		return &registry.KeyProviderAlreadyRegistered{ID: id}
	}
	l.providers[id] = keyProvider
	return nil
}

func (l *lockingRegistry) RegisterMethod(method method.Factory) error {
	l.lock.Lock()
	defer l.lock.Unlock()

	id := method.ID()
	if err := id.Validate(); err != nil {
		return &registry.InvalidMethod{Method: method, Cause: err}
	}
	if previousMethod, ok := l.methods[id]; ok {
		return &registry.MethodAlreadyRegistered{ID: id, CurrentMethod: method, PreviousMethod: previousMethod}
	}
	l.methods[id] = method
	return nil
}

func (l *lockingRegistry) GetKeyProvider(id keyprovider.ID) (keyprovider.Factory, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()
	provider, ok := l.providers[id]
	if !ok {
		return nil, &registry.KeyProviderNotFound{ID: id}
	}
	return provider, nil
}

func (l *lockingRegistry) GetMethod(id method.ID) (method.Factory, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()
	foundMethod, ok := l.methods[id]
	if !ok {
		return nil, &registry.MethodNotFound{ID: id}
	}
	return foundMethod, nil
}
