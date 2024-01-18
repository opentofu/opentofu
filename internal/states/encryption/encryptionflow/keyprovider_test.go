package encryptionflow

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
)

// an easy-to-understand example key provider with no random behaviours

const testingKeyProviderName = "testingkp"

func registerTestingKeyProvider() {
	_ = RegisterKeyProvider(KeyProviderMetadata{
		Name:            testingKeyProviderName,
		Constructor:     newTestingKeyProvider,
		ConfigValidator: validateTestingKeyProviderConfig,
	})
}

type testingKeyProvider struct{}

func newTestingKeyProvider() (KeyProvider, error) {
	return &testingKeyProvider{}, nil
}

func (t *testingKeyProvider) ProvideKey(info *EncryptionInfo, configuration *encryptionconfig.Config) ([]byte, error) {
	phrase, ok := configuration.KeyProvider.Config["testphrase"]
	if !ok || phrase == "" {
		return nil, errors.New("incomplete configuration - field 'testphrase' missing or empty")
	}

	var seed string
	if info.KeyProvider == nil {
		seed = "a real key provider might need something like this"
		// we do this just so it can be tested. What you add here shows up in the encrypted state in plain text.
		info.KeyProvider = &KeyProviderInfo{
			Name: testingKeyProviderName,
			Config: map[string]string{
				"seed": seed,
			},
		}
	} else {
		// we are decrypting, and got back what we set when we encrypted
		seed = info.KeyProvider.Config["seed"]
		logging.HCLogger().Trace("we are decrypting and got back our seed", "seed", seed)
	}

	// fake encryption key = base64(phrase + seed)
	key := base64.StdEncoding.EncodeToString([]byte(phrase + seed))
	return []byte(key), nil
}

func validateTestingKeyProviderConfig(k encryptionconfig.KeyProviderConfig) error {
	phrase, ok := k.Config["testphrase"]
	if !ok || phrase == "" {
		return errors.New("field 'testphrase' missing or empty")
	}

	if len(k.Config) > 1 {
		return errors.New("unexpected additional configuration fields, only 'testphrase' is allowed for this key provider")
	}

	return nil
}

// another key provider that produces errors and keys on demand

const testingErrorKeyProviderName = "testingerrorkp"

func registerTestingErrorKeyProvider() {
	_ = RegisterKeyProvider(KeyProviderMetadata{
		Name:            testingErrorKeyProviderName,
		Constructor:     newTestingErrorKeyProvider,
		ConfigValidator: validateTestingErrorKeyProviderConfig,
	})
}

type testingErrorKeyProvider struct{}

func newTestingErrorKeyProvider() (KeyProvider, error) {
	return &testingErrorKeyProvider{}, nil
}

func (t *testingErrorKeyProvider) ProvideKey(info *EncryptionInfo, configuration *encryptionconfig.Config) ([]byte, error) {
	errorMessage, ok := configuration.KeyProvider.Config["error"]
	if ok {
		return nil, errors.New(errorMessage)
	}

	key, _ := configuration.KeyProvider.Config["key"]
	return []byte(key), nil
}

func validateTestingErrorKeyProviderConfig(k encryptionconfig.KeyProviderConfig) error {
	return nil
}

// tests

func TestRegisterKeyProvider_Errors(t *testing.T) {
	err := RegisterKeyProvider(KeyProviderMetadata{})
	expectErr(t, err, errors.New("invalid metadata: cannot register a key provider with empty name"))

	err = RegisterKeyProvider(KeyProviderMetadata{
		Name:            "constructor_missing",
		Constructor:     nil,
		ConfigValidator: validateTestingErrorKeyProviderConfig,
	})
	expectErr(t, err, errors.New("invalid metadata: Constructor and ConfigValidator are mandatory when registering a key provider"))

	err = RegisterKeyProvider(KeyProviderMetadata{
		Name:            "validator_missing",
		Constructor:     newTestingErrorKeyProvider,
		ConfigValidator: nil,
	})
	expectErr(t, err, errors.New("invalid metadata: Constructor and ConfigValidator are mandatory when registering a key provider"))

	// using a name that is already registered
	err = RegisterKeyProvider(KeyProviderMetadata{
		Name:            testingErrorKeyProviderName,
		Constructor:     newTestingErrorKeyProvider,
		ConfigValidator: validateTestingErrorKeyProviderConfig,
	})
	expectErr(t, err, errors.New("duplicate registration for key provider \""+testingErrorKeyProviderName+"\""))
}
