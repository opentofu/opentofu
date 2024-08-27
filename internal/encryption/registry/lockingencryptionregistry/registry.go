// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package lockingencryptionregistry

import (
	"sync"

	"github.com/terramate-io/opentofulib/internal/encryption/keyprovider"
	"github.com/terramate-io/opentofulib/internal/encryption/method"
	"github.com/terramate-io/opentofulib/internal/encryption/registry"
)

// New returns a new encryption registry that locks for parallel access.
func New() registry.Registry {
	return &lockingRegistry{
		lock:      &sync.RWMutex{},
		providers: map[keyprovider.ID]keyprovider.Descriptor{},
		methods:   map[method.ID]method.Descriptor{},
	}
}

type lockingRegistry struct {
	lock      *sync.RWMutex
	providers map[keyprovider.ID]keyprovider.Descriptor
	methods   map[method.ID]method.Descriptor
}

func (l *lockingRegistry) RegisterKeyProvider(keyProvider keyprovider.Descriptor) error {
	l.lock.Lock()
	defer l.lock.Unlock()

	id := keyProvider.ID()
	if err := id.Validate(); err != nil {
		return &registry.InvalidKeyProviderError{KeyProvider: keyProvider, Cause: err}
	}
	if _, ok := l.providers[id]; ok {
		return &registry.KeyProviderAlreadyRegisteredError{ID: id}
	}
	l.providers[id] = keyProvider
	return nil
}

func (l *lockingRegistry) RegisterMethod(method method.Descriptor) error {
	l.lock.Lock()
	defer l.lock.Unlock()

	id := method.ID()
	if err := id.Validate(); err != nil {
		return &registry.InvalidMethodError{Method: method, Cause: err}
	}
	if previousMethod, ok := l.methods[id]; ok {
		return &registry.MethodAlreadyRegisteredError{ID: id, CurrentMethod: method, PreviousMethod: previousMethod}
	}
	l.methods[id] = method
	return nil
}

func (l *lockingRegistry) GetKeyProviderDescriptor(id keyprovider.ID) (keyprovider.Descriptor, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()
	provider, ok := l.providers[id]
	if !ok {
		return nil, &registry.KeyProviderNotFoundError{ID: id}
	}
	return provider, nil
}

func (l *lockingRegistry) GetMethodDescriptor(id method.ID) (method.Descriptor, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()
	foundMethod, ok := l.methods[id]
	if !ok {
		return nil, &registry.MethodNotFoundError{ID: id}
	}
	return foundMethod, nil
}
