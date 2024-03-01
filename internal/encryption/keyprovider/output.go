package keyprovider

type Output struct {
	EncryptionKey []byte `hcl:"encryption_key"`
	DecryptionKey []byte `hcl:"decryption_key"`
	Metadata      any
}
