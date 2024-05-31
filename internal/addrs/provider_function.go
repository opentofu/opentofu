// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"fmt"
	"strings"
)

// ProviderFunction is the address of a provider defined function.
type ProviderFunction struct {
	referenceable
	ProviderName  string
	ProviderAlias string
	Function      string
}

func (v ProviderFunction) String() string {
	if v.ProviderAlias != "" {
		return fmt.Sprintf("provider::%s::%s::%s", v.ProviderName, v.ProviderAlias, v.Function)
	}
	return fmt.Sprintf("provider::%s::%s", v.ProviderName, v.Function)
}

func (v ProviderFunction) UniqueKey() UniqueKey {
	return v // A ProviderFunction is its own UniqueKey
}

func (v ProviderFunction) uniqueKeySigil() {}

type Function struct {
	Namespaces []string
	Name       string
}

const (
	FunctionNamespaceProvider = "provider"
	FunctionNamespaceCore     = "core"
)

var FunctionNamespaces = []string{
	FunctionNamespaceProvider,
	FunctionNamespaceCore,
}

func ParseFunction(input string) Function {
	parts := strings.Split(input, "::")
	return Function{
		Name:       parts[len(parts)-1],
		Namespaces: parts[:len(parts)-1],
	}
}

func (f Function) String() string {
	return strings.Join(append(f.Namespaces, f.Name), "::")
}

func (f Function) IsNamespace(namespace string) bool {
	return len(f.Namespaces) > 0 && f.Namespaces[0] == namespace
}

func (f Function) AsProviderFunction() (pf ProviderFunction, err error) {
	if !f.IsNamespace(FunctionNamespaceProvider) {
		// Should always be checked ahead of time!
		panic("BUG: non-provider function " + f.String())
	}

	if len(f.Namespaces) == 2 {
		// provider::<name>::<function>
		pf.ProviderName = f.Namespaces[1]
	} else if len(f.Namespaces) == 3 {
		// provider::<name>::<alias>::<function>
		pf.ProviderName = f.Namespaces[1]
		pf.ProviderAlias = f.Namespaces[2]
	} else {
		return pf, fmt.Errorf("invalid provider function %q: expected provider::<name>::<function> or provider::<name>::<alias>::<function>", f)
	}
	pf.Function = f.Name
	return pf, nil
}
