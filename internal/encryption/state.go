package encryption

import "github.com/opentofu/opentofu/internal/encryption/method"

// This currently deals with raw bytes so it could be moved into it's own library and not depend explicitly on the OpenTofu codebase.
type StateEncryption interface {
	EncryptState([]byte) ([]byte, error)

	// Validation is performed that the decrypted []byte is some sort of json object
	DecryptState([]byte) ([]byte, error)
}

func NewState(methods []method.Method) StateEncryption {
	// TODO
	return nil
}
