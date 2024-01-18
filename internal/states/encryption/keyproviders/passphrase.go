package keyproviders

import (
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"
	"fmt"

	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionflow"
	"golang.org/x/crypto/pbkdf2"
)

func RegisterPassphraseKeyProvider() {
	must(encryptionflow.RegisterKeyProvider(encryptionflow.KeyProviderMetadata{
		Name:            encryptionconfig.KeyProviderPassphrase,
		Constructor:     newPassphrase,
		ConfigValidator: encryptionconfig.ValidateKPPassphraseConfig,
	}))
}

type passphraseImpl struct{}

func newPassphrase() (encryptionflow.KeyProvider, error) {
	return &passphraseImpl{}, nil
}

func (d *passphraseImpl) ProvideKey(info *encryptionflow.EncryptionInfo, configuration *encryptionconfig.Config) ([]byte, error) {
	if configuration.KeyProvider.Config == nil {
		return nil, fmt.Errorf(
			"configuration for key provider %s needs key_provider.config.passphrase set - "+
				"key_provider.config was not present", encryptionconfig.KeyProviderPassphrase)
	}
	phrase, ok := configuration.KeyProvider.Config["passphrase"]
	if !ok {
		return nil, fmt.Errorf(
			"configuration for key provider %s needs key_provider.config.passphrase set - "+
				"key_provider.config was present, but key_provider.config.passphrase was not",
			encryptionconfig.KeyProviderPassphrase)
	}
	if len(phrase) < 8 {
		logging.HCLogger().Warn("short passphrase - this is not secure")
	}

	salt, err := d.provideAndRecordSalt(info)
	if err != nil {
		return nil, err
	}

	key := pbkdf2.Key([]byte(phrase), salt, 4096, 32, sha512.New)
	return key, nil
}

func (d *passphraseImpl) provideAndRecordSalt(info *encryptionflow.EncryptionInfo) ([]byte, error) {
	if info.KeyProvider == nil {
		// we are encrypting - generate a new salt and put it in info

		salt := make([]byte, 16)
		if _, err := rand.Read(salt); err != nil {
			return nil, fmt.Errorf("could not generate salt: %w", err)
		}

		hexSalt := hex.EncodeToString(salt)
		info.KeyProvider = &encryptionflow.KeyProviderInfo{
			Name: encryptionconfig.KeyProviderPassphrase,
			Config: map[string]string{
				"salt": hexSalt,
			},
		}

		return salt, nil
	} else {
		// we are decrypting - read salt from info

		if info.KeyProvider.Config == nil {
			return nil, fmt.Errorf("state or plan corrupt or not suitable for key provider %s - "+
				"missing salt needed to recover the key", encryptionconfig.KeyProviderPassphrase)
		}
		hexSalt, ok := info.KeyProvider.Config["salt"]
		if !ok {
			return nil, fmt.Errorf("state or plan corrupt or not suitable for key provider %s - "+
				"missing salt needed to recover the key", encryptionconfig.KeyProviderPassphrase)
		}
		salt, err := hex.DecodeString(hexSalt)
		if err != nil {
			return nil, fmt.Errorf("state or plan corrupt or not suitable for key provider %s - "+
				"failed to decode salt needed to recover the key", encryptionconfig.KeyProviderPassphrase)
		}
		if len(salt) < 16 {
			return nil, fmt.Errorf("state or plan corrupt or not suitable for key provider %s - "+
				"failed to decode salt needed to recover the key", encryptionconfig.KeyProviderPassphrase)
		}
		return salt, nil
	}
}
