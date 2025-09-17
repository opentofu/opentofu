// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package providers

type SchemaFilter func(string) bool

// Factory is a function type that creates a new instance of a resource
// provider, or returns an error if that is impossible.
type Factory func(resources SchemaFilter, datasources SchemaFilter) (Interface, error)

// FactoryFixed is a helper that creates a Factory that just returns some given
// single provider.
//
// Unlike usual factories, the exact same instance is returned for each call
// to the factory and so this must be used in only specialized situations where
// the caller can take care to either not mutate the given provider at all
// or to mutate it in ways that will not cause unexpected behavior for others
// holding the same reference.
func FactoryFixed(p Interface) Factory {
	return func(_, _ SchemaFilter) (Interface, error) {
		return p, nil
	}
}
