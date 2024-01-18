package methods

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionflow"
)

const encryptionMethodPartial encryptionconfig.MethodName = "partial"

func RegisterPartialMethod() {
	must(encryptionflow.RegisterMethod(encryptionflow.MethodMetadata{
		Name:            encryptionMethodPartial,
		JsonOnly:        true,
		Constructor:     newPartial,
		ConfigValidator: validateEMPartialConfig,
	}))
}

type partialImpl struct{}

func newPartial() (encryptionflow.Method, error) {
	return &partialImpl{}, nil
}

func (p *partialImpl) Decrypt(encrypted encryptionflow.EncryptedDocument, info *encryptionflow.EncryptionInfo, configuration encryptionconfig.Config, keyProvider encryptionflow.KeyProvider) ([]byte, error) {
	key, err := keyProvider.ProvideKey(info, &configuration)
	if err != nil {
		return nil, err
	}

	salt, err := p.provideAndRecordSalt(info)
	if err != nil {
		return nil, err
	}

	processed, err := recurseProcessStringLeaves(map[string]any(encrypted), "(root)", func(v string) (string, error) {
		asBytes, err := fromMarkedBase64(v)
		if err != nil {
			return "", err
		}
		decrypted, err := decryptAESGCM(asBytes, key, salt)
		if err != nil {
			return "", fmt.Errorf("failed to decrypt: %s", err.Error())
		}
		return string(decrypted), nil
	})
	if err != nil {
		return nil, err
	}

	asJson, err := json.MarshalIndent(processed, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to encode processed state into json: %s", err.Error())
	}

	return asJson, nil
}

func (p *partialImpl) Encrypt(data []byte, info *encryptionflow.EncryptionInfo, configuration encryptionconfig.Config, keyProvider encryptionflow.KeyProvider) (encryptionflow.EncryptedDocument, error) {
	key, err := keyProvider.ProvideKey(info, &configuration)
	if err != nil {
		return nil, err
	}

	salt, err := p.provideAndRecordSalt(info)
	if err != nil {
		return nil, err
	}

	encrypted := make(map[string]any)
	err = json.Unmarshal(data, &encrypted)
	if err != nil {
		// TODO trace log error
		return nil, fmt.Errorf("failed to parse state from json: detailed error not shown for security reasons")
	}

	processed, err := recurseProcessStringLeaves(encrypted, "(root)", func(v string) (string, error) {
		encrypted, err := encryptAESGCM([]byte(v), key, salt, true)
		if err != nil {
			return "", fmt.Errorf("failed to encrypt: %s", err.Error())
		}
		return toMarkedBase64(encrypted), nil
	})
	if err != nil {
		return nil, err
	}

	return processed.(map[string]any), nil
}

func recurseProcessStringLeaves(currentAny any, path string, leafProcessor func(string) (string, error)) (any, error) {
	switch current := currentAny.(type) {
	case map[string]any:
		result := make(map[string]any)
		for k, v := range current {
			newV, err := recurseProcessStringLeaves(v, fmt.Sprintf("%s.%s", path, k), leafProcessor)
			if err != nil {
				return result, err
			}
			result[k] = newV
		}
		return result, nil
	case []any:
		result := make([]any, len(current))
		for i, v := range current {
			newV, err := recurseProcessStringLeaves(v, fmt.Sprintf("%s[%d]", path, i), leafProcessor)
			if err != nil {
				return result, err
			}
			result[i] = newV
		}
		return result, nil
	case string: // it's a string leaf
		newValue, err := leafProcessor(current)
		if err != nil {
			return "", fmt.Errorf("error processing string field '%s': %s", path, err.Error())
		}
		return newValue, nil
	default:
		return current, nil
	}
}

func (p *partialImpl) provideAndRecordSalt(info *encryptionflow.EncryptionInfo) ([]byte, error) {
	return provideAndRecordMethodSalt(info, encryptionMethodPartial)
}

func validateEMPartialConfig(m encryptionconfig.MethodConfig) error {
	if len(m.Config) > 0 {
		return errors.New("unexpected fields, this method needs no configuration")
	}
	// no config fields
	return nil
}
