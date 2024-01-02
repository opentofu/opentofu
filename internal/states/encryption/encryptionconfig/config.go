// Package encryptionconfig contains the data structures and constants to configure client-side state encryption.
package encryptionconfig

// Config is a configuration for transparent client-side state encryption.
//
// There can be more than one configuration with different encryption or key derivation methods.
//
// See also StateEncryptionConfigSchema() and StateDecryptionFallbackConfigSchema().
type Config struct {
	KeyProvider KeyProviderConfig `json:"key_provider"`

	Method EncryptionMethodConfig `json:"method"`

	// Required forces encryption operations to fail if they would
	// result in an unencrypted output.
	Required bool `json:"required"`
}

const (
	ConfigKeyDefault   = "default"   // a default for all remote states (backend + any remote_state data sources)
	ConfigKeyBackend   = "backend"   // for our own remote state backend
	ConfigKeyStatefile = "statefile" // for local state files
	ConfigKeyPlanfile  = "planfile"  // for plan files
)

// ConfigEnvJsonStructure is used to parse multiple named configurations from environment variables.
//
// The key either specifies a resource that accesses remote state, such as "terraform_remote_state.foo", or
// it is one of the pre-defined convenience keys ConfigKeyDefault, ConfigKeyBackend, ConfigKeyStatefile, or
// ConfigKeyPlanfile.
//
// If the key is "terraform_remote_state.foo", its value sets/overrides encryption configuration for
//
//	data "terraform_remote_state" "foo" {}
//
// For enumerated resources, the format is "terraform_remote_state.foo[17]" or "terraform_remote_state.foo[key]"
// (no quotes around the for_each key).
type ConfigEnvJsonStructure map[string]Config

// ConfigEnvName is the name of the environment variable used to configure encryption and decryption
//
// Set this environment variable to a json representation of ConfigEnvJsonStructure, or leave it unset/blank
// to disable encryption (default behaviour).
var ConfigEnvName = "TF_STATE_ENCRYPTION"

// FallbackConfigEnvName is the name of the environment variable used to configure fallback decryption
//
// Set this environment variable to a json representation of ConfigEnvJsonStructure, or leave it unset/blank
// in order to not supply any fallbacks (default behaviour).
//
// Note that decryption will always try the relevant configuration specified in TF_STATE_ENCRYPTION first.
// Only if decryption fails with that, it will try this configuration.
//
// Why is this useful?
//   - key rotation (put the old key here until all state has been migrated)
//   - decryption (leave TF_STATE_ENCRYPTION unset, but set this variable, and your state will be decrypted on next write)
var FallbackConfigEnvName = "TF_STATE_DECRYPTION_FALLBACK"

type KeyProviderName string

const (
	KeyProviderPassphrase KeyProviderName = "passphrase" // derive key from config field "passphrase"
	KeyProviderDirect     KeyProviderName = "direct"     // key is explicitly specified in config field "key"
)

type KeyProviderConfig struct {
	// Name specifies which key provider to use.
	Name KeyProviderName `json:"name"`

	// Config configures the key provider.
	//
	// The available values are key provider dependent.
	Config map[string]string `json:"config"`
}

type EncryptionMethodName string

const (
	EncryptionMethodFull EncryptionMethodName = "full" // full state encryption
)

type EncryptionMethodConfig struct {
	// Name specifies which encryption method to use.
	Name EncryptionMethodName `json:"name"`

	// Config configures the key provider.
	//
	// The available values are key provider dependent.
	Config map[string]string `json:"config"`
}
