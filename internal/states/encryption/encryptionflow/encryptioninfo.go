package encryptionflow

import "github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"

// EncryptionInfo is added to encrypted state or plans under the key EncryptionTopLevelJsonKey.
//
// If present, Flow knows that it is looking at encrypted state or plan.
type EncryptionInfo struct {
	// Version is currently always 1.
	Version int `json:"version"`

	// KeyProvider allows the key provider to store information in the encrypted state or plan.
	//
	// This field is optional, and in fact key providers are strongly encouraged NOT to use this,
	// as this will tie your encrypted state to a single key provider.
	KeyProvider *KeyProviderInfo `json:"key_provider,omitempty"`

	// Method tracks which method was used to encrypt this state or plan.
	Method MethodInfo `json:"method"`
}

type KeyProviderInfo struct {
	// Name identifies the key provider storing information.
	//
	// If KeyProviderInfo is present at all, this field must be filled.
	// This prevents other key providers from reading and misinterpreting the information.
	Name encryptionconfig.KeyProviderName `json:"name"`

	// Config contains key-value pairs for use by the key provider.
	//
	// Note that these are written to the state or plan in plain text, so you should not place keys here.
	Config map[string]string `json:"config,omitempty"`
}

type MethodInfo struct {
	// Name identifies the encryption method used to encrypt this state or plan.
	Name encryptionconfig.MethodName `json:"name"`

	// Config contains key-value pairs for use by the method.
	//
	// Note that these are written to the state or plan in plain text, so you should not place keys here.
	Config map[string]string `json:"config,omitempty"`
}
