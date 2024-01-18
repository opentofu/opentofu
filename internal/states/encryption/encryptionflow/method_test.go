package encryptionflow

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
)

// an easy-to-understand example method with no random behaviours
//
// this method "encrypts" using base64 on the input. It then adds a prefix "valid.", as a stand-in for
// a hash or mac. Any other prefix, and the decryption will fail to simulate a wrong hash/mac.

const testingMethodName = "testingmethod"

func registerTestingMethod() {
	_ = RegisterMethod(MethodMetadata{
		Name:            testingMethodName,
		JsonOnly:        false,
		Constructor:     newTestingMethod,
		ConfigValidator: validateTestingMethodConfig,
	})
}

type testingMethod struct{}

func newTestingMethod() (Method, error) {
	return &testingMethod{}, nil
}

func (t *testingMethod) Decrypt(encrypted EncryptedDocument, info *EncryptionInfo, configuration encryptionconfig.Config, keyProvider KeyProvider) ([]byte, error) {
	key, err := keyProvider.ProvideKey(info, &configuration)
	if err != nil {
		return []byte{}, err
	}

	payload, ok := encrypted["payload"]
	if !ok {
		return []byte{}, errors.New("field payload missing")
	}
	payloadStr, ok := payload.(string)
	if !ok {
		return []byte{}, errors.New("field payload is not a string")
	}

	if !strings.HasPrefix(payloadStr, "valid.") {
		return []byte{}, errors.New("fake hash of payload is not valid")
	}
	if !strings.HasPrefix(payloadStr, fmt.Sprintf("valid.%s.", key)) {
		return []byte{}, errors.New("fake encrypted with wrong key")
	}

	decrypted, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(payloadStr, fmt.Sprintf("valid.%s.", key)))
	if err != nil {
		return []byte{}, errors.New("decryption failed")
	}
	return decrypted, nil
}

func (t *testingMethod) Encrypt(payload []byte, info *EncryptionInfo, configuration encryptionconfig.Config, keyProvider KeyProvider) (EncryptedDocument, error) {
	key, err := keyProvider.ProvideKey(info, &configuration)
	if err != nil {
		return nil, err
	}

	encrypted := base64.StdEncoding.EncodeToString(payload)

	result := make(map[string]any)
	result["payload"] = fmt.Sprintf("valid.%s.%s", key, encrypted)

	return result, nil
}

func validateTestingMethodConfig(m encryptionconfig.MethodConfig) error {
	if len(m.Config) > 0 {
		return errors.New("unexpected fields, this method needs no configuration")
	}
	return nil
}

// another method we use for testing. This one reacts to the payload it receives to produce errors and results on demand.

const testingErrorMethodName = "testingerrormethod"

func registerTestingErrorMethod() {
	_ = RegisterMethod(MethodMetadata{
		Name:            testingErrorMethodName,
		JsonOnly:        true,
		Constructor:     newTestingErrorMethod,
		ConfigValidator: validateTestingErrorMethodConfig,
	})
}

type testingErrorMethod struct{}

func newTestingErrorMethod() (Method, error) {
	return &testingErrorMethod{}, nil
}

func (t *testingErrorMethod) Decrypt(encrypted EncryptedDocument, info *EncryptionInfo, configuration encryptionconfig.Config, keyProvider KeyProvider) ([]byte, error) {
	_, err := keyProvider.ProvideKey(info, &configuration)
	if err != nil {
		return []byte{}, err
	}

	decryptError, ok := encrypted["decryptError"]
	if ok {
		asStr, _ := decryptError.(string)
		return []byte{}, errors.New(asStr)
	}

	decryptValue, ok := encrypted["decryptValue"]
	asStr, _ := decryptValue.(string)
	return []byte(asStr), nil
}

func (t *testingErrorMethod) Encrypt(payload []byte, info *EncryptionInfo, configuration encryptionconfig.Config, keyProvider KeyProvider) (EncryptedDocument, error) {
	_, err := keyProvider.ProvideKey(info, &configuration)
	if err != nil {
		return nil, err
	}

	result := make(map[string]any)

	parsedPayload := make(map[string]any)
	err = json.Unmarshal(payload, &parsedPayload)
	if err != nil {
		return result, err
	}

	encryptError, ok := parsedPayload["encryptError"]
	if ok {
		asStr, _ := encryptError.(string)
		return result, errors.New(asStr)
	}

	encryptValue, ok := parsedPayload["encryptValue"]
	valueAsStr, _ := encryptValue.(string)

	encryptKey, ok := parsedPayload["encryptKey"]
	keyAsStr, _ := encryptKey.(string)

	result[keyAsStr] = valueAsStr

	return result, nil
}

func validateTestingErrorMethodConfig(m encryptionconfig.MethodConfig) error {
	return nil
}

// tests

func TestRegisterMethod_Errors(t *testing.T) {
	err := RegisterMethod(MethodMetadata{})
	expectErr(t, err, errors.New("invalid metadata: cannot register a method with empty name"))

	err = RegisterMethod(MethodMetadata{
		Name:            "constructor_missing",
		JsonOnly:        true,
		Constructor:     nil,
		ConfigValidator: validateTestingErrorMethodConfig,
	})
	expectErr(t, err, errors.New("invalid metadata: Constructor and ConfigValidator are mandatory when registering a method"))

	err = RegisterMethod(MethodMetadata{
		Name:            "validator_missing",
		JsonOnly:        true,
		Constructor:     newTestingErrorMethod,
		ConfigValidator: nil,
	})
	expectErr(t, err, errors.New("invalid metadata: Constructor and ConfigValidator are mandatory when registering a method"))

	// using a name that is already registered
	err = RegisterMethod(MethodMetadata{
		Name:            testingErrorMethodName,
		JsonOnly:        true,
		Constructor:     newTestingErrorMethod,
		ConfigValidator: validateTestingErrorMethodConfig,
	})
	expectErr(t, err, errors.New("duplicate registration for encryption method \"testingerrormethod\""))
}

func TestMethodIsJsonOnly_DefaultFalse(t *testing.T) {
	if methodIsJsonOnly("unregistered_name") {
		t.Error("unexpected default value")
	}
}
