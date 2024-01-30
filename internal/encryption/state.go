package encryption

// This currently deals with raw bytes so it could be moved into it's own library and not depend explicitly on the OpenTofu codebase.
type State interface {
	EncryptState([]byte) ([]byte, error)

	// Validation is performed that the decrypted []byte is some sort of json object
	DecryptState([]byte) ([]byte, error)
}
