package statemgr

import "github.com/opentofu/opentofu/internal/states/encryption/encryptionflow"

type SupportsEncryption interface {
	// UseEncryptionFlowBuilder provides the state with the encryption flow builder to use
	// for encryption / decryption.
	UseEncryptionFlowBuilder(builder encryptionflow.FlowBuilder)
}
