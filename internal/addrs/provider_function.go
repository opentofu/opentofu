package addrs

import (
	"fmt"
	"strings"
)

// ProviderFunction is the address of a provider defined function.
type ProviderFunction struct {
	referenceable
	Name     string
	Alias    string
	Function string
}

func (v ProviderFunction) String() string {
	if v.Alias != "" {
		return fmt.Sprintf("provider::%s::%s::%s", v.Name, v.Alias, v.Function)
	}
	return fmt.Sprintf("provider::%s::%s", v.Name, v.Function)
}

func (v ProviderFunction) UniqueKey() UniqueKey {
	return v // A ProviderFunction is its own UniqueKey
}

func (v ProviderFunction) uniqueKeySigil() {}

type Function struct {
	Namespaces []string
	Name       string
}

var (
	FunctionNamespaceProvider = "provider"
	FunctionNamespaceCore     = "core"
)

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
		panic(f.String())
	}

	if len(f.Namespaces) == 2 {
		// provider::<name>::<function>
		pf.Name = f.Namespaces[1]
	} else if len(f.Namespaces) == 3 {
		// provider::<name>::<alias>::<function>
		pf.Name = f.Namespaces[1]
		pf.Alias = f.Namespaces[2]
	} else {
		return pf, fmt.Errorf("invalid provider function %q: expected provider::<name>::<function> or provider::<name>::<alias>::<function>", f)
	}
	pf.Function = f.Name
	return pf, nil
}
