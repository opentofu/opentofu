package addrs

import (
	"fmt"
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
