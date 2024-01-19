// Package encryptionconfig contains the data structures and constants to configure client-side state encryption.
package encryptionconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Config is a configuration for transparent client-side state encryption.
//
// There can be more than one configuration with different encryption or key derivation methods.
type Config struct {
	KeyProvider KeyProviderConfig `json:"key_provider"`

	Method MethodConfig `json:"method"`

	// Enforced forces encryption operations to fail if they would
	// result in an unencrypted output.
	Enforced bool `json:"enforced"`
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
	ConfigKeyDefault   = "default"   // configuration for all remote states that do not have an explicit configuration
	ConfigKeyBackend   = "backend"   // when uploading the state to a remote backend
	ConfigKeyStatefile = "statefile" // for a locally stored state file
	ConfigKeyPlanfile  = "planfile"  // for a locally stored plan file
)

// ConfigEnvStructure is used to hold multiple named configurations from environment variables.
//
// For the map key, you can specify a resource this configuration is relevant for, or you can use one of the
// predefined keys to specify the base configuration for all resources.
//
// Predefined keys:
//
// You can use the following constants as map keys to provide base configuration:
//
//   - ConfigKeyDefault for all remote states that do not have an explicit configuration (including the backend)
//   - ConfigKeyBackend when uploading the state to a remote backend
//   - ConfigKeyStatefile for a locally stored state file
//   - ConfigKeyPlanfile for a locally stored plan file
//
// Explicit resources:
//
// You can specify an explicit configuration for a remote state data source (see terraform_remote_state). You can use
// this to specify a different encryption key or method when you want to read a state file from a different remote
// storage than the backend for the current project.
//
// If the key is "terraform_remote_state.foo", its value sets/overrides the encryption configuration for
//
//	data "terraform_remote_state" "foo" {}
//
// For indexed resources, the format is "terraform_remote_state.foo[17]" or "terraform_remote_state.foo[key]"
// (no quotes around the index).
//
// Use case:
//
// This is useful when you need to access information from another tofu environment. For example, you may want to
// segment your tofu setup into multiple projects for both security and to optimize the run time. One such example
// is Terragrunt. You may, for example, wish to obtain resources IDs for DNS entries from the project responsible for
// DNS configuration for the purpose of setting up a web server.
//
// When using shared state and shared encryption in this fashion, the author of the other project must pay attention
// to only expose information that is intended for the current environment in the state.
type ConfigEnvStructure map[string]Config

// ConfigEnvName is the name of the environment variable used to configure encryption and decryption as an alternative
// to providing the configuration in the .tf files directly.
//
// Set this environment variable to a JSON representation of ConfigEnvStructure, or leave it unset/blank
// to disable encryption (default behaviour). If you do not specify a configuration but "enforced" is set to true, tofu
// will refuse to function. If you specify an invalid JSON, the entire tofu run will fail regardless of the "enforced"
// setting.
//
// Note: With rare exceptions, you should avoid setting the state encryption environment variables in tests,
// as this may make tests depend on each other. See the comments on encryption.ParseEnvironmentVariables().
const ConfigEnvName = "TF_STATE_ENCRYPTION"

// FallbackConfigEnvName is the name of the environment variable used to configure fallback decryption of the state.
//
// OpenTofu will always try to decrypt the state with the primary key and method, and falls back to this key and
// method if it fails.
//
// You can use the fallback configuration for decrypting the state with the specified key and method, and then
// re-encrypt the state with the primary configuration as a means of key or method rollover. If you do not specify a
// primary configuration, the state will be decrypted unless you set the "enforced" flag to true, which prevents a
// decryption and results in a failure.
//
// Set this environment variable to a JSON representation of ConfigEnvStructure, or leave it unset/blank
// in order to not supply any fallbacks (default behaviour). If you specify an invalid JSON, the entire tofu run will
// fail regardless of the "enforced" setting.
//
// Note: With rare exceptions, you should avoid setting the state encryption environment variables in tests,
// as this may make tests depend on each other. See the comments on encryption.ParseEnvironmentVariables().
const FallbackConfigEnvName = "TF_STATE_DECRYPTION_FALLBACK"

// ConfigurationFromEnv parses the encryption configuration from the environment variable envName.
//
// If the provided environment variable is empty, nil will be returned without an error as an empty configuration
// means no encryption is desired.
func ConfigurationFromEnv(envName string) (ConfigEnvStructure, error) {
	envValue := os.Getenv(envName)
	if envValue == "" {
		return nil, nil
	}

	parsed, err := parseJsonStructure(envValue)
	if err != nil {
		return nil, fmt.Errorf("error parsing environment variable %s (%w)", envName, err)
	}

	return parsed, nil
}

func parseJsonStructure(jsonValue string) (ConfigEnvStructure, error) {
	parsed := make(ConfigEnvStructure)

	// This JSON decoder is needed to disallow unknown fields
	decoder := json.NewDecoder(strings.NewReader(jsonValue))
	// Avoid typos in configuration
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&parsed)
	if err != nil {
		return nil, errors.New("failed to parse encryption configuration, please check if your configuration is correct (not showing error because it may contain sensitive credentials)")
	}

	// we cannot validate just this part of the configuration - may need to be merged with
	// values from code first
	return parsed, nil
}
