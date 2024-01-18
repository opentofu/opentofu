package keyproviders

import (
	"encoding/hex"
	"fmt"

	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionflow"
)

func RegisterDirectKeyProvider() {
	must(encryptionflow.RegisterKeyProvider(encryptionflow.KeyProviderMetadata{
		Name:            encryptionconfig.KeyProviderDirect,
		Constructor:     newDirect,
		ConfigValidator: encryptionconfig.ValidateKPDirectConfig,
	}))
}

type directImpl struct{}

func newDirect() (encryptionflow.KeyProvider, error) {
	return &directImpl{}, nil
}

func (d *directImpl) ProvideKey(info *encryptionflow.EncryptionInfo, configuration *encryptionconfig.Config) ([]byte, error) {
	if configuration.KeyProvider.Config == nil {
		return nil, fmt.Errorf(
			"configuration for key provider %s needs key_provider.config.key set to a 64 character hexadecimal value "+
				"(32 byte key) - key_provider.config was not present", encryptionconfig.KeyProviderDirect)
	}
	hexKey, ok := configuration.KeyProvider.Config["key"]
	if !ok {
		return nil, fmt.Errorf(
			"configuration for key provider %s needs key_provider.config.key set to a 64 character hexadecimal value "+
				"(32 byte key) - key_provider.config was present, but key_provider.config.key was not",
			encryptionconfig.KeyProviderDirect)
	}
	if len(hexKey) != 64 {
		return nil, fmt.Errorf(
			"configuration for key provider %s needs key_provider.config.key set to a 64 character hexadecimal value "+
				"(32 byte key) - value was %d instead of 64 characters long",
			encryptionconfig.KeyProviderDirect, len(hexKey))
	}

	key, err := hex.DecodeString(hexKey)
	if err != nil {
		logging.HCLogger().Trace("failed to decode key from hex", "error", err.Error())
		return nil, fmt.Errorf(
			"configuration for key provider %s needs key_provider.config.key set to a 64 character hexadecimal value "+
				"(32 byte key) - failed to decode hex value - omitting detailed error for security reasons",
			encryptionconfig.KeyProviderDirect)
	}

	return key, nil
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
