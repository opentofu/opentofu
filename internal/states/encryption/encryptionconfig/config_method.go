package encryptionconfig

import "fmt"

type MethodName string

// TODO remove this in a follow-up PR (when the method is actually introduced)
const (
	MethodFull MethodName = "full" // full state encryption
)

type MethodConfig struct {
	// Name specifies which encryption method to use.
	Name MethodName `json:"name"`

	// Config configures the key provider.
	//
	// The available values are key provider dependent.
	Config map[string]string `json:"config"`
}

// Validate checks the configuration after it has been merged from all sources.
func (m MethodConfig) Validate() error {
	validator, ok := methodConfigValidators.get(m.Name)
	if !ok || validator == nil {
		return fmt.Errorf("error in configuration for encryption method %s: no registered encryption method with this name", m.Name)
	}

	if err := validator(m); err != nil {
		return fmt.Errorf("error in configuration for encryption method %s: %s", m.Name, err.Error())
	}

	return nil
}
