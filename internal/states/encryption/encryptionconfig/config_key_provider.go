package encryptionconfig

import "fmt"

type KeyProviderName string

// TODO remove this in a follow-up PR (when the key providers are actually introduced)
const (
	KeyProviderPassphrase KeyProviderName = "passphrase" // derive key from config field "passphrase"
	KeyProviderDirect     KeyProviderName = "direct"     // key is explicitly specified in config field "key"
)

type KeyProviderConfig struct {
	// Name specifies which key provider to use.
	Name KeyProviderName `json:"name"`

	// Config configures the key provider.
	//
	// The available values are key provider dependent.
	Config map[string]string `json:"config"`
}

// Validate checks the configuration after it has been merged from all sources.
func (k KeyProviderConfig) Validate() error {
	validator, ok := keyProviderConfigValidators.get(k.Name)
	if !ok || validator == nil {
		return fmt.Errorf("error in configuration for key provider %s (no registered key provider with this name)", k.Name)
	}

	if err := validator(k); err != nil {
		return fmt.Errorf("error in configuration for key provider %s (%w)", k.Name, err)
	}

	return nil
}
