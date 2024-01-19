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
	Meta `json:"-"`

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

// Meta holds the metadata about a config structure.
type Meta struct {
	Source Source `json:"-"`
	Key    Key    `json:"-"`
}

type ConfigMap map[Meta]Config

// Merge merges the configuration map for the specified config key according to the following rules:
//
//   - If no configuration exists for the specified key, the default configuration is read from the environment.
//   - Otherwise, the configuration from the environment and from HCL for the specific config key is merged, where the
//     values from the environment take precedence.
//
// When the configuration from HCL and the environment is merged, a deep merge is performed.
//
// If no default or specific configuration is found, the function returns nil.
func (c ConfigMap) Merge(configKey Key) (*Config, error) {
	configOrNil := func(configs ConfigMap, meta Meta) *Config {
		conf, ok := configs[meta]
		if ok {
			return &conf
		} else {
			return nil
		}
	}

	merged := mergeConfigs(
		configOrNil(c, Meta{SourceEnv, KeyDefault}),
		configOrNil(c, Meta{SourceHCL, configKey}),
		configOrNil(c, Meta{SourceEnv, configKey}),
	)

	injectDefaultNamesIfNotSet(merged)

	if merged != nil {
		if err := merged.Validate(); err != nil {
			return merged, fmt.Errorf("invalid configuration after merge (%w)", err)
		}
	}

	return merged, nil
}

// Key is a value type indicating that a string is intended to be used as a configuration key. You can use this type to
// specify a resource this configuration is relevant for, or you can use one of the predefined keys to specify the base
// configuration for all resources.
//
// Predefined keys:
//
// You can use the following constants as map keys to provide base configuration:
//
//   - KeyDefault for all remote states that do not have an explicit configuration (including the backend)
//   - KeyBackend when uploading the state to a remote backend
//   - KeyStatefile for a locally stored state file
//   - KeyPlanfile for a locally stored plan file
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
type Key string

const (
	KeyDefault   Key = "default"   // configuration for all remote states that do not have an explicit configuration
	KeyBackend   Key = "backend"   // when uploading the state to a remote backend
	KeyStatefile Key = "statefile" // for a locally stored state file
	KeyPlanfile  Key = "planfile"  // for a locally stored plan file
)

// ConfigEnvStructure is used to hold multiple named configurations from environment variables. See also the
// documentation for Key.
type ConfigEnvStructure map[Key]Config

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

	for key, _ := range parsed {
		var item = parsed[key]
		item.Key = key
		item.Source = SourceEnv
		parsed[key] = item
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
