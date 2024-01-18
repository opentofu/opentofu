package methods

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionflow"
)

func RegisterFullMethod() {
	must(encryptionflow.RegisterMethod(encryptionflow.MethodMetadata{
		Name:            encryptionconfig.MethodFull,
		JsonOnly:        false,
		Constructor:     newFull,
		ConfigValidator: encryptionconfig.ValidateEMFullConfig,
	}))
}

type fullImpl struct{}

func newFull() (encryptionflow.Method, error) {
	return &fullImpl{}, nil
}

func (f *fullImpl) Decrypt(encrypted encryptionflow.EncryptedDocument, info *encryptionflow.EncryptionInfo, configuration encryptionconfig.Config, keyProvider encryptionflow.KeyProvider) ([]byte, error) {
	markedPayload, ok := encrypted["payload"]
	if !ok {
		return nil, fmt.Errorf("no field 'payload' in encrypted data")
	}

	payload, err := fromMarkedBase64(markedPayload)
	if err != nil {
		return nil, err
	}

	key, err := keyProvider.ProvideKey(info, &configuration)
	if err != nil {
		return nil, err
	}

	salt, err := f.provideAndRecordSalt(info)
	if err != nil {
		return nil, err
	}

	decrypted, err := decryptAESGCM(payload, key, salt)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt with method %s: %s", encryptionconfig.MethodFull, err.Error())
	}

	return decrypted, nil
}

func (f *fullImpl) Encrypt(data []byte, info *encryptionflow.EncryptionInfo, configuration encryptionconfig.Config, keyProvider encryptionflow.KeyProvider) (encryptionflow.EncryptedDocument, error) {
	key, err := keyProvider.ProvideKey(info, &configuration)
	if err != nil {
		return nil, err
	}

	salt, err := f.provideAndRecordSalt(info)
	if err != nil {
		return nil, err
	}

	encrypted, err := encryptAESGCM(data, key, salt, false)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt with method %s: %s", encryptionconfig.MethodFull, err.Error())
	}

	result := make(map[string]any)
	result["payload"] = toMarkedBase64(encrypted)

	return result, nil
}

func (f *fullImpl) provideAndRecordSalt(info *encryptionflow.EncryptionInfo) ([]byte, error) {
	return provideAndRecordMethodSalt(info, encryptionconfig.MethodFull)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
