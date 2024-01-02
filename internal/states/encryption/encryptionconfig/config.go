// Package encryptionconfig contains the data structures and constants to configure client-side state encryption.
package encryptionconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/opentofu/opentofu/internal/logging"
	"os"
	"strings"
)

// Config is a configuration for transparent client-side state encryption.
//
// There can be more than one configuration with different encryption or key derivation methods.
type Config struct {
	KeyProvider KeyProviderConfig `json:"key_provider"`

	Method EncryptionMethodConfig `json:"method"`

	// Required forces encryption operations to fail if they would
	// result in an unencrypted output.
	Required bool `json:"required"`
}

func (c Config) Validate() error {
	if err := c.KeyProvider.Validate(); err != nil {
		return err
	}
	if err := c.Method.Validate(); err != nil {
		return err
	}
	return nil
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
//
// Note: With rare exceptions, you should avoid setting the state encryption environment variables in tests,
// as this may make tests depend on each other. See the comments on encryption.ParseEnvironmentVariables().
var ConfigEnvName = "TF_STATE_ENCRYPTION"

// EncryptionConfigurationsFromEnv parses the encryption configuration from the environment variable configured
// in ConfigEnvName.
//
// It is not an error if the environment variable is unset or empty, that just means no encryption configuration
// is provided via environment.
func EncryptionConfigurationsFromEnv() (ConfigEnvJsonStructure, error) {
	return parseEnvJsonStructure(ConfigEnvName, "encryption configuration")
}

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
//
// Note: With rare exceptions, you should avoid setting the state encryption environment variables in tests,
// as this may make tests depend on each other. See the comments on encryption.ParseEnvironmentVariables().
var FallbackConfigEnvName = "TF_STATE_DECRYPTION_FALLBACK"

// FallbackConfigurationsFromEnv parses the decryption fallback configuration from the environment variable
// configured in FallbackConfigEnvName.
//
// It is not an error if the environment variable is unset or empty, that just means no decryption fallback
// configuration is provided via environment.
func FallbackConfigurationsFromEnv() (ConfigEnvJsonStructure, error) {
	return parseEnvJsonStructure(FallbackConfigEnvName, "fallback decryption configuration")
}

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

// NameValid checks that the name has been registered correctly.
//
// This is an early check, unlike Validate().
func (k KeyProviderConfig) NameValid() error {
	validator, ok := keyProviderConfigValidation[k.Name]
	if !ok || validator == nil {
		return fmt.Errorf("error in configuration for key provider %s: no registered key provider with this name", k.Name)
	}
	return nil
}

// Validate checks the configuration after it has been merged from all sources.
func (k KeyProviderConfig) Validate() error {
	if err := k.NameValid(); err != nil {
		return err
	}

	validator := keyProviderConfigValidation[k.Name]
	if err := validator(k); err != nil {
		return fmt.Errorf("error in configuration for key provider %s: %s", k.Name, err.Error())
	}

	return nil
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

// NameValid checks that the name has been registered correctly.
//
// This is an early check, unlike Validate().
func (m EncryptionMethodConfig) NameValid() error {
	validator, ok := encryptionMethodConfigValidation[m.Name]
	if !ok || validator == nil {
		return fmt.Errorf("error in configuration for encryption method %s: no registered encryption method with this name", m.Name)
	}
	return nil
}

// Validate checks the configuration after it has been merged from all sources.
func (m EncryptionMethodConfig) Validate() error {
	if err := m.NameValid(); err != nil {
		return err
	}

	validator := encryptionMethodConfigValidation[m.Name]

	if err := validator(m); err != nil {
		return fmt.Errorf("error in configuration for encryption method %s: %s", m.Name, err.Error())
	}

	return nil
}

func parseJsonStructure(jsonValue string) (ConfigEnvJsonStructure, error) {
	parsed := make(ConfigEnvJsonStructure)

	decoder := json.NewDecoder(strings.NewReader(jsonValue))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&parsed)
	if err != nil {
		logging.HCLogger().Trace("json parse error", "details", err.Error())
		return nil, errors.New("json parse error, wrong structure, or unknown fields - details omitted for security reasons (may contain key related settings)")
	}

	// we cannot validate just this part of the configuration - may need to be merged with
	// values from code first
	return parsed, nil
}

func parseEnvJsonStructure(envName string, what string) (ConfigEnvJsonStructure, error) {
	envValue := os.Getenv(envName)
	if envValue == "" {
		return nil, nil
	}

	parsed, err := parseJsonStructure(envValue)
	if err != nil {
		return nil, fmt.Errorf("error parsing %s from environment variable %s: %s", what, envName, err.Error())
	}

	return parsed, nil
}
