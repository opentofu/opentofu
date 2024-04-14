package addrs

import "fmt"

// ProviderFunction is the address of an input variable.
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

// Absolute converts the receiver into an absolute address within the given
// module instance.
func (v ProviderFunction) Absolute(m ModuleInstance) AbsProviderFunctionInstance {
	return AbsProviderFunctionInstance{
		Module:   m,
		Provider: v,
	}
}

// AbsProviderFunctionInstance is the address of an input variable within a
// particular module instance.
type AbsProviderFunctionInstance struct {
	Module   ModuleInstance
	Provider ProviderFunction
}

// ProviderFunction returns the absolute address of the input variable of the
// given name inside the receiving module instance.
func (m ModuleInstance) ProviderFunction(name string) AbsProviderFunctionInstance {
	return AbsProviderFunctionInstance{
		Module: m,
		Provider: ProviderFunction{
			Name: name,
		},
	}
}

func (v AbsProviderFunctionInstance) String() string {
	if len(v.Module) == 0 {
		return v.Provider.String()
	}

	return fmt.Sprintf("%s.%s", v.Module.String(), v.Provider.String())
}

func (v AbsProviderFunctionInstance) UniqueKey() UniqueKey {
	return absProviderFunctionInstanceUniqueKey(v.String())
}

type absProviderFunctionInstanceUniqueKey string

func (k absProviderFunctionInstanceUniqueKey) uniqueKeySigil() {}
